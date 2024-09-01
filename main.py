import requests
import time
import random
from loguru import logger
from threading import Thread
import argparse
import json


def update_username_list(username):
    with open("listtocheck.txt", "r") as f:
        usernames = [line.strip() for line in f if line.strip()]
    if username in usernames:
        usernames.remove(username)
        with open("listtocheck.txt", "w") as f:
            f.write("\n".join(usernames))


with open("config.json", "r") as f:
    config = json.load(f)

method = config["method"]


def check_username(username, token, headers):
    try:

        json_data = {"username": username}
        if method == "friends":
            response = requests.post(
                "https://discord.com/api/v9/users/@me/pomelo-attempt",
                headers=headers,
                json=json_data,
            )
        else:
            response = requests.patch(
                "https://discord.com/api/v9/users/@me", headers=headers, json=json_data
            )
        response_json = response.json()
        if (
            response.status_code == 400
            and response_json["message"] == "Invalid Form Body"
        ):
            errors = response_json["errors"]
            if "username" in errors:
                error_codes = [error["code"] for error in errors["username"]["_errors"]]
                if "USERNAME_ALREADY_TAKEN" in error_codes:
                    return "taken"
            elif (
                "PASSWORD_DOES_NOT_MATCH" in str(errors)
                or response_json["taken"] is not True
            ):
                return "not_taken"
        elif response.status_code == 200 and "taken" in response_json:
            if response_json["taken"] is not False:
                return "taken"
            elif response_json["taken"] is not True:
                return "not_taken"
        elif response.status_code == 401:
            if response_json.get("code") == 40001:
                return "40001"
            else:
                return "unauthorized"

        elif "retry_after" in response_json:
            return "rate_limited", response_json["retry_after"]
        else:
            return "unknown_error", response_json
    except requests.exceptions.RequestException as e:
        return "connection_error"


def load_file(file_name):
    with open(file_name, "r") as f:
        return [line.strip() for line in f if line.strip()]


def append_to_file(file_name, content):
    with open(file_name, "a") as f:
        f.write(content + "\n")


from threading import Lock


class Token:
    def __init__(self, token):
        self.token = token
        self.sleep_until = time.time() + random.uniform(1, 3)
        self.lock = Lock()
        self.in_use = False

    def set_sleep_until(self, sleep_until):
        with self.lock:
            self.sleep_until = sleep_until

    def get_sleep_until(self):
        with self.lock:
            return self.sleep_until

    def set_in_use(self, in_use):
        with self.lock:
            self.in_use = in_use

    def get_in_use(self):
        with self.lock:
            return self.in_use


def get_best_token(tokens):
    available_tokens = [token for token in tokens if not token.get_in_use()]
    if not available_tokens:
        return None
    best_token = available_tokens[0]
    for token in available_tokens[1:]:
        if token.get_sleep_until() < best_token.get_sleep_until():
            best_token = token
    return best_token


class Worker(Thread):
    def __init__(self, tokens, usernames_queue, lock):
        Thread.__init__(self)
        self.tokens = tokens
        self.usernames_queue = usernames_queue
        self.lock = lock
        self.checked_usernames = []

    def get_headers(self, token):
        headers = {
            "authority": "discord.com",
            "accept": "*/*",
            "accept-language": "ru,en;q=0.9,en-GB;q=0.8,en-US;q=0.7",
            "content-type": "application/json",
            "origin": "https://discord.com",
            "referer": "https://discord.com/channels/@me",
        }
        headers["authorization"] = token
        return headers

    def run(self):
        while not self.usernames_queue.empty():
            with self.lock:
                if self.usernames_queue.empty():
                    break
                username = self.usernames_queue.get()

                best_token = get_best_token(self.tokens)
                if best_token is None:
                    logger.warning(
                        f"Thread {self.name}: No available tokens. Terminating thread."
                    )
                    return

            while time.time() < best_token.get_sleep_until():
                time.sleep(0.1)

            headers = self.get_headers(best_token.token)
            result = check_username(username, best_token.token, headers)

            if result == "taken":
                logger.info(
                    f"Thread {self.name}: Username {username} is already taken."
                )
                append_to_file("bad.txt", username)
                best_token.set_sleep_until(time.time() + random.uniform(4, 6))
            elif result == "not_taken":
                logger.info(f"Thread {self.name}: Username {username} is available.")
                append_to_file("good.txt", username)
                best_token.set_sleep_until(time.time() + random.uniform(4, 6))

            elif result == "connection_error":
                logger.warning(
                    f"Thread {self.name}: Connection error. Retrying in 10 seconds."
                )
                best_token.set_sleep_until(time.time() + 10)

            elif result == "40001":
                if method == "friends":
                    logger.warning(
                        f"Thread {self.name}: Token {best_token.token} is not suitable for the friends method and has been removed."
                    )
                    self.tokens.remove(best_token)
                    best_token.set_sleep_until(time.time() + 10)

            elif result == "unauthorized":
                logger.warning(
                    f"Thread {self.name}: Token {best_token.token} is unauthorized and has been removed."
                )
                self.tokens.remove(best_token)
                best_token.set_sleep_until(time.time() + 10)

            elif result == "unknown_error":
                error_json = result[1]
                logger.error(f"Unknown error: {error_json}")

            elif result[0] == "rate_limited":
                sleep_time = result[1] + random.uniform(0.5, 1.2)
                logger.warning(
                    f"Thread {self.name}: Token {best_token.token} is rate limited. Sleeping for {sleep_time} seconds before the next request."
                )
                best_token.set_sleep_until(time.time() + sleep_time)

            if result in ["taken", "not_taken", "unknown_error"]:
                update_username_list(username)
            else:
                self.usernames_queue.put(username)
            best_token.set_in_use(False)


def main():
    parser = argparse.ArgumentParser(description="Check Discord usernames.")
    parser.add_argument(
        "-t",
        "--threads",
        type=int,
        default=1,
        help="Number of threads to use (default: 1)",
    )
    args = parser.parse_args()

    threads = args.threads
    tokens, usernames = load_file("tokens.txt"), load_file("listtocheck.txt")

    if not usernames:
        logger.error("The file listtocheck.txt is empty")
        input()
        return
    if not tokens:
        logger.error("The file tokens.txt is empty")
        input()
        return

    valid_tokens = []
    for token in tokens:
        logger.info(f"Checking token {token}")
        headers = {"authorization": token}
        connection_error = True

        while connection_error:
            try:
                if method == "friends":
                    response = requests.post(
                        "https://discord.com/api/v9/users/@me/pomelo-attempt",
                        headers=headers,
                        json={"username": ""},
                    )
                else:
                    response = requests.patch(
                        "https://discord.com/api/v9/users/@me",
                        headers=headers,
                        json={"username": ""},
                    )

                response_json = response.json()
                connection_error = False

                if "retry_after" in response_json:
                    logger.error(f"Token {token} is rate limited")
                elif response.status_code == 401:
                    logger.error(f"Token {token} is unauthorized and has been removed")
                elif "USERNAME_ALREADY_TAKEN" in str(
                    response_json
                ) or "BASE_TYPE_BAD_LENGTH" in str(response_json):
                    logger.success(f"Token {token} is ready to work.")
                    valid_tokens.append(Token(token))
                elif "USERNAME_TOO_MANY_USERS" in str(response_json):
                    logger.error(f"Token {token} cannot set a username without a tag")

            except requests.exceptions.RequestException:
                logger.warning(
                    f"Connection error when checking token: {token}. Retrying in 10 seconds."
                )
                time.sleep(10)

    if not valid_tokens:
        logger.error("Work finished. No valid tokens")
        input()
        return

    if threads > len(valid_tokens):
        threads = len(valid_tokens)
        logger.warning(
            f"The number of threads has been reduced to {threads} to match the number of valid tokens"
        )

    lock = Lock()

    from queue import Queue

    usernames_queue = Queue()
    for username in usernames:
        usernames_queue.put(username)

    workers = []
    for i in range(threads):
        worker = Worker(valid_tokens, usernames_queue, lock)
        workers.append(worker)
        worker.start()

    for worker in workers:
        worker.join()

    logger.success("No more usernames to check.")
    input()


if __name__ == "__main__":
    main()
