# build stage
FROM golang:1.16rc1-alpine AS build-env
RUN apk --no-cache add build-base git gcc
ADD . /src
RUN cd /src && go build -v -ldflags "-s -w"

# final stage
FROM alpine
WORKDIR /app
COPY --from=build-env /src/glider /app/
ENTRYPOINT ["./glider"]
