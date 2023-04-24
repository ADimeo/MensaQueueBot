# Mensa Queue Bot

This is a telegram bot written in go that allows you to record and receive the current length of the Griebnitzsee mensa queue, as well as see the menus currently on offer.

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
- Allows users to receive the mensa menu currently on offer
    - Both via request and push
    - Includes settings, including weekday and timeslot selection
- Allows to define messages that should be sent to users the next time they interact with the bot
    - In praxis, this is mostly used for changelogs
    - To define a new message to be sent, edit `changelog.psv`
- That's about it




## Repo Structure
This repo is at the point where starting to work on features requires institutional knowledge that is currently not explicitly documented. In general it is well structured, but not all code is documented as thoroughly as it could be. If you want to contribute feel free to request a guided tour from a maintainer.

### Folders and modules
- `analysis` includes python scripts and published queue length data. It is not relevant for bot development
- `db/migrations` contains just that. We use golang-migrate to apply these
- `db_connectors` act as "model", and implement all queries against the DB
- `deployment` contains ansible scripts and server/docker-compose configs used for deployment
- `mensa_scraper` is a relatively independent module that is responsible for both getting the current mensa menus, storing them in the db using `db_connectors`, and sending them out to users both when menus change and when requested.
- `queue_length_illustrations` contains images that are sent to bot users to illustrate the different queue lengths
- `static` contains an html file that is used to modify bot settings. It needs to be hosted somewhere
- `telegram_connector` is responsible for all interaction with telegram
- `utils` contains utility functions

### Further files of interest
- `changelog.psv` is a csv (except with pipes as a separator) that defines messages to be sent to users. Pleaes keep IDs incrementing one by one
- `mensa_locations` contains links to illustrations, and needs to be consistend with the keyboards defined in `telegram_connector/keyboards`
- `db_connectors/db_utilities.go` contains the `DB_VERSION` variable, which is used to decide whether migrations should be applied. Only increment it, and keep it consistent with `db/migrations`

### Debug mode
During development the environment variable `MENSA_QUEUE_BOT_DEBUG_MODE` can be set. If it is set to a telegram user ID that ID will receive debug messages when running `egrap_test.go`. Additionally, if set to any value at all, it will alter behaviour:
- Mensa length reports will be allowed at all times
- The mensa scraper will run every minute of every day instead of every 10 minutes while the mensa is open
- Different default values may be set, e.g. for mensa menu preferences


# Development setup
The following steps can be taken to run a fully functional MensaQueueBot locally. Feel free to replace steps where you are more comfortable with alternative solutions
1. Install go
2. Create a new telegram bot as described by [telegram documentation](https://core.telegram.org/bots/features#botfather)
3. Install a proxy service such as [ngrok](https://ngrok.com/)
4. Set the following environment variables in a shell via `export`
    - `MENSA_QUEUE_BOT_DB_PATH` to any path, it's where the DB for reports wil lbe
    - `MENSA_QUEUE_BOT_PERSONAL_TOKEN` to an arbitrary string. This string hides the endpoint which accepts requests from telegrams servers. It's a security feature that doesn't need to be user for a development deployment
    - `MENSA_QUEUE_BOT_TELEGRAM_TOKEN` to the token you received when creating your bot
    - `MENSA_QUEUE_BOT_DEBUG_MODE` can optionally be set to any value. If it is set a couple of things work differently, e.g. you can report mensa lengths at any time. Also used during testing to define the telegram ID of the dev that wants to receive debug messages.
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
