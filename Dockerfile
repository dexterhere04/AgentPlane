FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY agentplane /agentplane
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

EXPOSE 3001

ENTRYPOINT ["/docker-entrypoint.sh"]
