### Run locally using Docker
```
docker build -t nulog-service ./

docker run -e NATS_SERVER_URL="nats://host.docker.internal:4222" -d --name nulog-service nulog-service
```