FROM node:22-alpine AS web-build

WORKDIR /src/web

COPY web/package*.json ./
RUN npm install

COPY web/ ./
RUN npm run build


FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /agentplane ./cmd/server


FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=build /agentplane /agentplane
COPY --from=build /src/migrations /migrations
COPY --from=web-build /src/web/dist /app/web/dist

COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

EXPOSE 3001

ENTRYPOINT ["/docker-entrypoint.sh"]
