# build stage
FROM golang:alpine AS build-env
RUN apk --no-cache add build-base git gcc
ADD . /src
RUN cd /src && go build -v -ldflags "-s -w"

# final stage
FROM alpine
RUN apk -U upgrade --no-cache && \
    apk add --no-cache bind-tools ca-certificates
WORKDIR /app
COPY --from=build-env /src/glider /app/
ENTRYPOINT ["./glider"]
