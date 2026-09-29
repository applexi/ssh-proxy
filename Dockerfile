FROM golang:latest AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -o /out/proxy .

FROM debian:bookworm-slim

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /srv
COPY --from=build /out/proxy /usr/local/bin/proxy

EXPOSE 2022
ENTRYPOINT ["proxy", "-cfg", "config.yaml"]
