# Discord Username Checker

This is a Python script that allows you to check the availability of Discord usernames. It utilizes multiple tokens, multithreading, and handles rate limits, connection errors, and unauthorized tokens.

## Requirements

- Python 3.7 or higher
- Dependencies listed in the `requirements.txt` file

## Installation

1. Clone the repository:

   ```bash
   git clone https://github.com/y0f/discord-user-checker.git

2. pip install -r requirements.txt


## Configuration

1. Open the `config.json` file and specify the desired method (e.g., friends) for checking usernames.

- `friends`: checks by submitting a Discord friend request (the currently working method).

2. Prepare your tokens and wordlists:

- Create a `tokens.txt` file and specify your Discord token in it. **Make sure you don't share your token with anyone.**

- If you want to filter your own wordlists, use the `helper.py` by running `python helper.py`, the script is included in the repository. Follow the instructions provided in the script to split lines or filter word length in the wordlist(s). 

## Usage

Run the following command to start checking the availability of usernames:

python main.py

The script will start checking the availability of usernames in the wordlists using the provided token. The results will be stored in separate files: `available.txt` and `unavailable.txt`.

## License

This project is licensed under the MIT License.

## Acknowledgments

The loguru library for flexible logging: https://github.com/Delgan/loguru

## Disclaimer
This script is for educational purposes only. Use it at your own risk. The author is not responsible for any consequences that may result from using this script.
