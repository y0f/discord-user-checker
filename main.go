package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const banner = `
            ██╗   ██╗ ██████╗  ███████╗
            ╚██╗ ██╔╝ ██╔═══██╗██╔════╝
             ╚████╔╝  ██║   ██║█████╗
              ╚██╔╝   ██║   ██║██╔══╝
               ██║    ╚██████╔╝██║
               ╚═╝     ╚═════╝ ╚═╝
          discord username checker — by y0f
`

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[91m"
	colorGreen  = "\033[92m"
	colorYellow = "\033[93m"
	colorCyan   = "\033[96m"
	colorDim    = "\033[2m"
)

func ts() string { return time.Now().Format("15:04:05") }

func logInfo(format string, a ...any) {
	fmt.Printf("%s%s | %sINF%s | %s\n", colorDim, ts(), colorCyan, colorReset, fmt.Sprintf(format, a...))
}

func logSuccess(format string, a ...any) {
	fmt.Printf("%s%s | %sSUC%s | %s\n", colorDim, ts(), colorGreen, colorReset, fmt.Sprintf(format, a...))
}

func logWarning(format string, a ...any) {
	fmt.Printf("%s%s | %sWRN%s | %s\n", colorDim, ts(), colorYellow, colorReset, fmt.Sprintf(format, a...))
}

func logError(format string, a ...any) {
	fmt.Printf("%s%s | %sERR%s | %s\n", colorDim, ts(), colorRed, colorReset, fmt.Sprintf(format, a...))
}

type Config struct {
	Method string `json:"method"`
}

func loadConfig() Config {
	data, err := os.ReadFile("config.json")
	if err != nil {
		logError("Failed to read config.json: %v", err)
		os.Exit(1)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		logError("Failed to parse config.json: %v", err)
		os.Exit(1)
	}
	return cfg
}

func loadFile(name string) []string {
	f, err := os.Open(name)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func appendToFile(name, content string) {
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, content)
}

var listMu sync.Mutex

func updateUsernameList(username string) {
	listMu.Lock()
	defer listMu.Unlock()
	lines := loadFile("listtocheck.txt")
	var remaining []string
	for _, l := range lines {
		if l != username {
			remaining = append(remaining, l)
		}
	}
	os.WriteFile("listtocheck.txt", []byte(strings.Join(remaining, "\n")), 0644)
}

type Token struct {
	mu         sync.Mutex
	value      string
	sleepUntil time.Time
	inUse      bool
}

func NewToken(value string) *Token {
	return &Token{
		value:      value,
		sleepUntil: time.Now().Add(time.Duration(1000+rand.Intn(2000)) * time.Millisecond),
	}
}

func (t *Token) SleepUntil() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sleepUntil
}

func (t *Token) SetSleepUntil(ts time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sleepUntil = ts
}

func (t *Token) InUse() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.inUse
}

func (t *Token) SetInUse(v bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inUse = v
}

type checkResult struct {
	status     string
	retryAfter float64
	raw        string
}

func buildHeaders(token string) map[string]string {
	return map[string]string{
		"authority":       "discord.com",
		"accept":          "*/*",
		"accept-language": "en-US,en;q=0.9",
		"content-type":    "application/json",
		"origin":          "https://discord.com",
		"referer":         "https://discord.com/channels/@me",
		"authorization":   token,
	}
}

func checkUsername(username, token, method string) checkResult {
	payload, _ := json.Marshal(map[string]string{"username": username})

	var reqURL, httpMethod string
	if method == "friends" {
		reqURL = "https://discord.com/api/v9/users/@me/pomelo-attempt"
		httpMethod = http.MethodPost
	} else {
		reqURL = "https://discord.com/api/v9/users/@me"
		httpMethod = http.MethodPatch
	}

	req, err := http.NewRequest(httpMethod, reqURL, bytes.NewReader(payload))
	if err != nil {
		return checkResult{status: "connection_error"}
	}
	for k, v := range buildHeaders(token) {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return checkResult{status: "connection_error"}
	}
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return checkResult{status: "connection_error"}
	}

	if resp.StatusCode == 400 {
		msg, _ := body["message"].(string)
		if msg == "Invalid Form Body" {
			errors, _ := body["errors"].(map[string]any)
			if usernameErrs, ok := errors["username"].(map[string]any); ok {
				if errList, ok := usernameErrs["_errors"].([]any); ok {
					for _, e := range errList {
						if em, ok := e.(map[string]any); ok {
							if c, _ := em["code"].(string); c == "USERNAME_ALREADY_TAKEN" {
								return checkResult{status: "taken"}
							}
						}
					}
				}
			} else {
				errBytes, _ := json.Marshal(errors)
				if strings.Contains(string(errBytes), "PASSWORD_DOES_NOT_MATCH") {
					return checkResult{status: "not_taken"}
				}
				if taken, ok := body["taken"].(bool); ok && !taken {
					return checkResult{status: "not_taken"}
				}
			}
		}
	}

	if resp.StatusCode == 200 {
		if taken, ok := body["taken"].(bool); ok {
			if taken {
				return checkResult{status: "taken"}
			}
			return checkResult{status: "not_taken"}
		}
	}

	if resp.StatusCode == 401 {
		if code, _ := body["code"].(float64); int(code) == 40001 {
			return checkResult{status: "40001"}
		}
		return checkResult{status: "unauthorized"}
	}

	if ra, ok := body["retry_after"].(float64); ok {
		return checkResult{status: "rate_limited", retryAfter: ra}
	}

	raw, _ := json.Marshal(body)
	return checkResult{status: "unknown_error", raw: string(raw)}
}

func getBestToken(tokens []*Token) *Token {
	var best *Token
	for _, t := range tokens {
		if t.InUse() {
			continue
		}
		if best == nil || t.SleepUntil().Before(best.SleepUntil()) {
			best = t
		}
	}
	return best
}

func worker(id int, tokens *[]*Token, tokensMu *sync.Mutex, queue chan string, method string, wg *sync.WaitGroup, queueLock *sync.Mutex) {
	defer wg.Done()

	for {
		queueLock.Lock()
		var username string
		select {
		case u, ok := <-queue:
			if !ok {
				queueLock.Unlock()
				return
			}
			username = u
		default:
			queueLock.Unlock()
			return
		}

		tokensMu.Lock()
		best := getBestToken(*tokens)
		if best == nil {
			tokensMu.Unlock()
			queueLock.Unlock()
			logWarning("Worker %d: No available tokens. Terminating.", id)
			return
		}
		best.SetInUse(true)
		tokensMu.Unlock()
		queueLock.Unlock()

		for time.Now().Before(best.SleepUntil()) {
			time.Sleep(100 * time.Millisecond)
		}

		result := checkUsername(username, best.value, method)

		requeue := true
		switch result.status {
		case "taken":
			logInfo("Worker %d: Username %s is already taken.", id, username)
			appendToFile("bad.txt", username)
			best.SetSleepUntil(time.Now().Add(time.Duration(4000+rand.Intn(2000)) * time.Millisecond))
			requeue = false

		case "not_taken":
			logSuccess("Worker %d: Username %s is available.", id, username)
			appendToFile("good.txt", username)
			best.SetSleepUntil(time.Now().Add(time.Duration(4000+rand.Intn(2000)) * time.Millisecond))
			requeue = false

		case "connection_error":
			logWarning("Worker %d: Connection error. Retrying in 10 seconds.", id)
			best.SetSleepUntil(time.Now().Add(10 * time.Second))

		case "40001":
			if method == "friends" {
				logWarning("Worker %d: Token %s is not suitable for the friends method and has been removed.", id, best.value)
				tokensMu.Lock()
				removeToken(tokens, best)
				tokensMu.Unlock()
			}
			best.SetSleepUntil(time.Now().Add(10 * time.Second))

		case "unauthorized":
			logWarning("Worker %d: Token %s is unauthorized and has been removed.", id, best.value)
			tokensMu.Lock()
			removeToken(tokens, best)
			tokensMu.Unlock()
			best.SetSleepUntil(time.Now().Add(10 * time.Second))

		case "rate_limited":
			sleep := result.retryAfter + 0.5 + rand.Float64()*0.7
			logWarning("Worker %d: Token %s is rate limited. Sleeping for %.1f seconds.", id, best.value, sleep)
			best.SetSleepUntil(time.Now().Add(time.Duration(sleep*1000) * time.Millisecond))

		case "unknown_error":
			logError("Unknown error: %s", result.raw)
		}

		if !requeue {
			updateUsernameList(username)
		} else {
			queue <- username
		}
		best.SetInUse(false)
	}
}

func removeToken(tokens *[]*Token, target *Token) {
	for i, t := range *tokens {
		if t == target {
			*tokens = append((*tokens)[:i], (*tokens)[i+1:]...)
			return
		}
	}
}

var alphaOnly = regexp.MustCompile(`[^a-zA-Z]`)
var alphaExact = regexp.MustCompile(`^[a-zA-Z]+$`)

func helperSplitLines(filename string) {
	path := filepath.Join("wordlists", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		logError("Error: File '%s' not found.", path)
		return
	}
	words := strings.Fields(string(data))
	if len(words) == 0 {
		logError("Error: File '%s' is empty.", path)
		return
	}
	var filtered []string
	for _, w := range words {
		clean := alphaOnly.ReplaceAllString(w, "")
		if clean != "" {
			filtered = append(filtered, clean)
		}
	}
	os.WriteFile(path, []byte(strings.Join(filtered, "\n")+"\n"), 0644)
	logInfo("Words have been split into separate lines.")
}

func helperFilterWords(filename string, length int) {
	path := filepath.Join("wordlists", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		logError("Error: File '%s' not found.", path)
		return
	}
	pattern := regexp.MustCompile(fmt.Sprintf(`^[a-zA-Z]{%d}$`, length))
	words := strings.Fields(string(data))
	var filtered []string
	for _, w := range words {
		if pattern.MatchString(w) {
			filtered = append(filtered, w)
		}
	}
	os.WriteFile(path, []byte(strings.Join(filtered, "\n")+"\n"), 0644)
	logInfo("Words have been filtered to length %d.", length)
}

func helperFilterAndSaveByLength(filename string) {
	path := filepath.Join("wordlists", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		logError("Error: File '%s' not found.", path)
		return
	}
	groups := make(map[int][]string)
	for _, w := range strings.Fields(string(data)) {
		if alphaExact.MatchString(w) {
			groups[len(w)] = append(groups[len(w)], w)
		}
	}
	for length, words := range groups {
		out := filepath.Join("wordlists", fmt.Sprintf("%dchar.txt", length))
		os.WriteFile(out, []byte(strings.Join(words, "\n")+"\n"), 0644)
	}
	logInfo("Words have been filtered by length and saved to separate files.")
}

func runHelper() {
	fmt.Println(`
    Option 1: Splits words into separate lines by removing any non-alphabetic characters.
    Option 2: Filters the words in the file to a specific length input by the user.
    Option 3: Filters the words in the file by their length and saves them into separate files named according to their length
    (i.e., 4-letter words will be saved in a file named '4char.txt', 5-letter words in '5char.txt', and so on)
    This can be useful when you have a gigantic wordlist.`)
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter the name of the file to process: ")
	filename, _ := reader.ReadString('\n')
	filename = strings.TrimSpace(filename)

	logInfo("Select an option:")
	logInfo("1. Split words into separate lines")
	logInfo("2. Filter words by length")
	logInfo("3. Filter words and save by length")

	fmt.Print("Enter your option (1/2/3): ")
	option, _ := reader.ReadString('\n')
	option = strings.TrimSpace(option)

	switch option {
	case "1":
		helperSplitLines(filename)
	case "2":
		fmt.Print("Enter the length to filter words: ")
		lenStr, _ := reader.ReadString('\n')
		lenStr = strings.TrimSpace(lenStr)
		length, err := strconv.Atoi(lenStr)
		if err != nil {
			logError("Invalid number.")
			return
		}
		helperFilterWords(filename, length)
	case "3":
		helperFilterAndSaveByLength(filename)
	default:
		logError("Invalid option.")
	}
}

func validateToken(token, method string) string {
	payload, _ := json.Marshal(map[string]string{"username": ""})

	var reqURL, httpMethod string
	if method == "friends" {
		reqURL = "https://discord.com/api/v9/users/@me/pomelo-attempt"
		httpMethod = http.MethodPost
	} else {
		reqURL = "https://discord.com/api/v9/users/@me"
		httpMethod = http.MethodPatch
	}

	req, err := http.NewRequest(httpMethod, reqURL, bytes.NewReader(payload))
	if err != nil {
		return "connection_error"
	}
	req.Header.Set("authorization", token)
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "connection_error"
	}
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "connection_error"
	}

	if _, ok := body["retry_after"]; ok {
		return "rate_limited"
	}
	if resp.StatusCode == 401 {
		return "unauthorized"
	}

	raw, _ := json.Marshal(body)
	rawStr := string(raw)

	if strings.Contains(rawStr, "USERNAME_ALREADY_TAKEN") || strings.Contains(rawStr, "BASE_TYPE_BAD_LENGTH") {
		return "valid"
	}
	if strings.Contains(rawStr, "USERNAME_TOO_MANY_USERS") {
		return "too_many_users"
	}

	return "unknown"
}

func main() {
	fmt.Println(banner)

	threads := flag.Int("t", 1, "Number of threads to use")
	helperMode := flag.Bool("helper", false, "Run the wordlist helper utility")
	flag.Parse()

	if *helperMode {
		runHelper()
		return
	}

	cfg := loadConfig()
	method := cfg.Method

	tokenLines := loadFile("tokens.txt")
	usernames := loadFile("listtocheck.txt")

	if len(usernames) == 0 {
		logError("The file listtocheck.txt is empty")
		fmt.Scanln()
		return
	}
	if len(tokenLines) == 0 {
		logError("The file tokens.txt is empty")
		fmt.Scanln()
		return
	}

	var validTokens []*Token
	for _, tok := range tokenLines {
		logInfo("Checking token %s", tok)
		for {
			status := validateToken(tok, method)
			switch status {
			case "connection_error":
				logWarning("Connection error when checking token: %s. Retrying in 10 seconds.", tok)
				time.Sleep(10 * time.Second)
				continue
			case "rate_limited":
				logError("Token %s is rate limited", tok)
			case "unauthorized":
				logError("Token %s is unauthorized and has been removed", tok)
			case "valid":
				logSuccess("Token %s is ready to work.", tok)
				validTokens = append(validTokens, NewToken(tok))
			case "too_many_users":
				logError("Token %s cannot set a username without a tag", tok)
			}
			break
		}
	}

	if len(validTokens) == 0 {
		logError("Work finished. No valid tokens")
		fmt.Scanln()
		return
	}

	numThreads := *threads
	if numThreads > len(validTokens) {
		numThreads = len(validTokens)
		logWarning("The number of threads has been reduced to %d to match the number of valid tokens", numThreads)
	}

	queue := make(chan string, len(usernames))
	for _, u := range usernames {
		queue <- u
	}

	var wg sync.WaitGroup
	var tokensMu sync.Mutex
	var queueLock sync.Mutex

	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go worker(i+1, &validTokens, &tokensMu, queue, method, &wg, &queueLock)
	}

	wg.Wait()
	logSuccess("No more usernames to check.")
	fmt.Scanln()
}
