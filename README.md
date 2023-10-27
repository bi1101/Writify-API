# WRITIFY_API


## Deploy on your VPS

1. copy tls certificate and private key into the root of the project folder

```bash
sudo cp /etc/letsencrypt/live/api2.ieltsscience.fun/fullchain.pem .
sudo cp /etc/letsencrypt/live/api2.ieltsscience.fun/privkey.pem .
```

2. run the docker-compose file

```bash
docker-compose up --build -d
```

***if you want to run it without docker you just need to specify this env variables. then run the executable***
```bash
export CERT_FILE=fullchain.pem
export KEY_FILE=privkey.pem
```
