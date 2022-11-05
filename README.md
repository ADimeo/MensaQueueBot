# Mensa Queue Bot

This is a telegram bot written in go that allows you to record the current length of the Griebnitzsee mensa queue.

> Oh shit. Oh shit. Oh shit.
>
> -- <cite>Enthusiastic user feedback</cite>


## Features
- Allows users to report current queue length
    - Stores these reports without allowing direct inference of who reported it
    - Reports are stored in a sqlite database
        - Users can collect internetpoints for their reports
- Allows users to request the current queue length
    - Reports to users are graphic, and contain both historical and current data
- Allows to define messages that should be sent to users the next time they interact with the bot
    - In praxis, this is mostly used for changelogs
    - To define a new message to be sent, edit `changelog.psv`
- That's about it




## Structure
- `queue_length_illustrations` contains images that are sent to bot users to illustrate the different queue lengths
- `mensa_locations.json` defines the different mensa locations, and should contain direct links to the images within `queue_length_illustrations`. These should also be consistent with the buttons defined in `keyboard.json`
- `keyboard.json` defines the buttons that are shown to users
- `emoji_list` contains selective non-aggressive emoji that can be used for whatever
- `changelog.psv` is a csv (except with pipes as a separator) that defines messages to be sent to users. Pleaes keep IDs incrementing one by one


- `db_utilities.go` implements a number of base functions that can be useful for all db related tasks
- `db_connector.go` implements database logic related to storing and retrieving actual queue length reports
- `changelog_db_connector.go` implements database logic related to tracking which users are aware of which changes, and what changelogs should still be sent out
- `internetpoints_db_connector.go` implements database logic related to users collecting internetpoints


- `points_handler.go` handles all requests that relate to point collection, e.g. signup, explanation, and number of points
- `reports_handler.go` handles all requests that relate to reporting of queue length
- `requests_handler.go` handles all requests that relate to requesting the queue length

- `telegram_connector.go` implements most of the telegram-interaction related logic

- `storage.go` contains functions that either act as static variables, or have encoded some knowledge that really should be stored somewhere else in a proper program. Basically, a catchall for functions that are hacky
- `main.go` implements the rest

- `deployment` folder contains
        - A `Caddyfile` tht defines [web server](https://caddyserver.com/) configuration
        - A `docker-compose` file that allows for relatively simple deployment of a server + reverse proxy setup
        - A `deploy-mensa-queue.yaml` file for use with [ansible](https://www.ansible.com/), because I'm supposed to be learning that right now
        - A `pull_csv.yaml` and `pull_db.yaml` ansible script, which pull only the reports, or the entitre database respectively, from within the remote docker folder ot the local filesystem



# Development setup
The following steps can be taken to run a fully functional MensaQueueBot locally. Feel free to replace steps where you are more comfortable with alternative solutions
1. Install go
2. Create a new telegram bot as described by [telegram documentation](https://core.telegram.org/bots/features#botfather)
3. Install a proxy service such as [ngrok](https://ngrok.com/)
4. Set the following environment variables in a shell via `export`
    - `MENSA_QUEUE_BOT_DB_PATH` to any path, it's where the DB for reports wil lbe
    - `MENSA_QUEUE_BOT_PERSONAL_TOKEN` to an arbitrary string. This string hides the endpoint which accepts requests from telegrams servers. It's a security feature that doesn't need to be user for a development deployment
    - `MENSA_QUEUE_BOT_TELEGRAM_TOKEN` to the token you received when creating your bot
    - `MENSA_QUEUE_BOT_DEBUG_MODE` can optionally be set to any value. If it is set a couple of things work differently, e.g. you can report mensa lengths at any time
5. Allow telegrams servers to connect to your development server by telling them where you are
    - Start the proxy service, e.g. with `ngrok http 8080` in a second shell
    - Tell telegrams servers with `curl -F "url=[url ngrok displays to you]/[string you set as MENSA_QUEUE_BOT_PERSONAL_TOKEN/"  "https://api.telegram.org/bot[your MENSA_QUEUE_BOT_PERSONAL_TOKEN/setWebhook"`
        - So if your token is `ABCDE` the final request is to `https://api.telegram.org/botABCDE/setWebhook`
6. In the same shell where you set the environment variables run `go run .`


## Deployment
1. `mv deployment/.env-template deployment/.env` and modify all variables within it
2. Advise telegram where your bot will be hosted, e.g. via `curl -F "url=https://your.url.example.com/long-random-string-defined-as-MENSA_QUEUE_BOT_PERSONAL_TOKEN/"  "https://api.telegram.org/bot<telegram-token-provided-by-botfather>/setWebhook"`
3. Build the docker container on the machine you want to run it on with `docker build -t mensaqueuebot .`
4. `cd deployment && docker-compose --env-file .env up --build` to the bot server and a reverse proxy

Steps 3. and 4. can be automated away with the `deploy-mensa-queue.yaml` ansible file that is provided in the `deployment` folder.

## Extracting Data
This assumes that the deployment is identical to the one described above.

Data is stored in an sqlite file within a docker volume created by docker-compose. To download it to your machine follow these steps:

1. ssh into your server
2. Copy the report file from the docker volume to your home directory via `sudo cp /var/lib/docker/volumes/deployment_db_data/_data/queue_database.db /home/your-user/databases/queue_database.db`
3. Copy the file from the remote system to your system by using rsync ip-of-your-system:~/queue_database.db .

You now have a local sqlite3 file. You can view or edit it in a variety of ways, including the [DB browser for SQLITE](https://sqlitebrowser.org/), or the [command line shell for sqlite](https://www.sqlite.org/cli.html)

Mensa queue length reports are in the queueReports table. You can extract them to csv by running `sqlite3 -header -csv cheue_database.db "select time, queueLength from queueReports" > queueReports.csv`

All steps can be automated using the `pull_db.yaml` ansible file that is provided in the `deployment` folder.
