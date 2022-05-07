# Mensa Queue Bot

This is a minimalist telegram bot written in go that allows you to record the current length of the Griebnitzsee mensa queue.

## Features
- Allows users to report current queue length
	- Stores these reports without allowing direct inference of who reported it
	- Reports are stored in a sqlite database
- Allows users to request the current queue length
- That's about it



## Structure
- `queue_length_illustrations` contains images that are sent to bot users to illustrate the different queue lengths
- `mensa_locations.json` defines the different mensa locations, and should contain direct links to the images within `queue_length_illustrations`. These should also be consistent with the buttons defined in `keyboard.json`
- `keyboard.json` defines the buttons that are shown to users
- `emoji_list` contains selective non-aggressive emoji that can be used for whatever

- `db_connector.go` implements most of the database related logic
- `telegram_connecor.go` implements most of the telegram-interaction related logic
- `main.go` implements the rest
- `deployment` folder contains
        - A `Caddyfile` tht defines [web server](https://caddyserver.com/) configuration
        - A `docker-compose` file that allows for relatively simple deployment of a server + reverse proxy setup
        - A `deploy-mensa-queue.yaml` file for use with [ansible](https://www.ansible.com/), because I'm supposed to be learning that right now


## Deployment
1. `mv  /docker-compose/.env-template /docker-compose.env` and modify all variables within it
2. Advise telegram where your bot will be hosted, e.g. via `curl -F "url=https://your.url.example.com/long-random-string-defined-as-MENSA_QUEUE_BOT_PERSONAL_TOKEN/"  "https://api.telegram.org/bot<telegram-token-provided-by-botfather>/setWebhook"`
3. Build the docker container locally with `docker build -t mensaqueuebot .`
4. `cd deployment && docker-compose --env-file .env up --build` to the bot server and a reverse proxy

Steps 3. and 4. can be automated away with the `deploy-mensa-queue.yaml` ansible file that is provided.
