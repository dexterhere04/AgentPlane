FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /mockapi ./cmd/mockapi

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /mockapi /mockapi
EXPOSE 3002
ENTRYPOINT ["/mockapi"]
