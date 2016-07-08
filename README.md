
send docker container cpu and memory usage to elasticsearch

```
docker build -t docker-es-agent .

docker run -d -e MARATHON_APP_ID=app1 --name nginx -P nginx

docker run -d -e LOGGER_ADDR=127.0.0.1:8080 -e LOGGER_INDEX=logstash-docker -e LOG_LEVEL=debug docker-es-agent
```
