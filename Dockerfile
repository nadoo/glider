# build stage
FROM golang:alpine AS build-env
RUN apk --no-cache add build-base git gcc
ADD . /src
RUN cd /src && go build -v -i -ldflags "-s -w"

# final stage
FROM alpine
WORKDIR /app
COPY --from=build-env /src/glider /app/
ENTRYPOINT ["./glider"]
