# RSSNOTES

rssnotes is a nostr relay that functions as an rss to nostr bridge by creating nostr profiles and notes for RSS feeds. rssnotes is a read only relay.  rssnotes is a fork of [rsslay](https://github.com/piraces/rsslay).

## Features
- Convert RSS feeds into Nostr profiles.
- Creates a pubkey, npubkey and QR code for each RSS feed profile that you can use to follow the RSS feed on nostr.
- The rssnotes relay also has its own pubkey.  The rssnotes relay pubkey automatically follows all of the rss feed profiles. So if you login to nostr as the rssnotes relay you will see all of your RSS feeds.
- Option to import and export multiple RSS feeds at once using an opml file.
- Option to automatically remove old notes.
- Search bar
- Relay logs exposed on the /log path.
- Using [khatru](https://github.com/fiatjaf/khatru)

## Screenshot

![alt text](screenshots/rssnotes-github.png)

## Run the relay using docker compose
Prerequisites:
- [Docker](https://docs.docker.com/get-docker/)
- [Docker Compose](https://docs.docker.com/compose/install/) - preinstalled on many systems nowadays

1. Create a new `rssnotes` folder and cd into it.
```bash
mkdir rssnotes
cd rssnotes
```
2. Create a folder called `db`.

3. Create a file called `logfile.log`.

4. Create a file called `docker-compose.yml`.

5. Copy and paste the contents from the [sample.docker-compose.yml](https://github.com/trinidz/rssnotes/blob/main/sample.docker-compose.yml) file into your `docker-compose.yml` file. Save and exit the file.

6. Create a file called `.env`.  

7. Copy and paste the contents of the [sample.env](https://github.com/trinidz/rssnotes/blob/main/sample.env) file into your `.env` file.

8. Modify the contents of your `.env` file. Add values for the following REQUIRED environment variables. 
- **RELAY_PRIVKEY** --- Use a nostr key generator to create a new set of nostr private and public keys for the relay. DO NOT USE your own existing nostr keys.  The relay will use these keys to follow all of your rss feeds and for other background tasks. 
- **RELAY_PUBKEY** --- acquired from the new private key created above.
- **RANDOM_SECRET** --- This is used to generate the nostr public/private keys for the rss feeds.  This should be a randomly generated string at least 20 characters long.
- **RELAY_URL**  --- the URL of your relay ex.: myrssrelay.com.  This is only used for display on the relay's main page.  It does not affect your relays actual URL.

9. Save and exit the `.env` file.

10. Create a file called `seedrelays.json`. 

11. Copy and paste the contents from the [sample.seedrelays.json](https://github.com/trinidz/rssnotes/blob/main/sample.seedrelays.json) file into your `seedrelays.json` file. Save and exit the file.

12. Run `docker-compose up -d` while in the `rssnotes` directory. This will start the rssnotes container in the background.

## Run the relay as a service on your system
1. Clone the repo and cd into the repo folder.
```bash
git clone https://github.com/trinidz/rssnotes
cd rssnotes
```
2. Download the correct [rssnotes released binary](https://github.com/trinidz/rssnotes/releases) for your system into the rssnotes folder.

3. Copy and rename the binary and the necessary files:
```bash
# The binary format is simillar to rssnotes-rx.x.x-ostype-arch. Change the cp command to match the specific binary you downloaded.
cp rssnotes-rx.x.x-ostype-arch rssnotes
```
```bash
cp sample.env .env
```
```bash
cp sample.seedrelays.json seedrelays.json
```
4. Open the .env file and add values for the following REQUIRED environment variables. 
- **RELAY_PRIVKEY** --- Use a nostr key generator to create a new set of nostr private and public keys for the relay. DO NOT USE your own existing nostr keys.  The relay will use these keys to follow all of your rss feeds and for other background tasks. 
- **RELAY_PUBKEY** --- acquired from the new private key created above.
- **RELAY_URL**  --- the URL of your relay ex.: myrssrelay.com.  This is only used for display on the relay's main page.  It does not affect your relays actual URL.
- **RANDOM_SECRET** --- This is used to generate the nostr public/private keys for the rss feeds.  This should be a randomly generated string at least 20 characters long.

5. The remaining variables in the .env file are optional.

6. Create a systemd service file:

```bash
sudo nano /etc/systemd/system/rssnotes.service
```

7.  Add the following contents:

```ini
[Unit]
Description=RSSNotes Relay Service
After=network.target

[Service]
User=myuser
Group=myuser
ExecStart=/home/myuser/rssnotes/rssnotes
WorkingDirectory=/home/myuser/rssnotes
Restart=always
MemoryLimit=2G

[Install]
WantedBy=multi-user.target
```
8. Replace /home/myuser/ with the actual paths where the files are stored.

9. Reload systemd to recognize the new service:

```bash
sudo systemctl daemon-reload
```

10. Start the service:

```bash
sudo systemctl start rssnotes
```

11. Enable the service to start on boot:

```bash
sudo systemctl enable rssnotes
```

12. Go to http://localhost:3334 in your browser.