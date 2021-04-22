# Build Stage
FROM golang:1.16-alpine AS build-env

ADD . /src

RUN apk --no-cache add build-base git gcc \
    && cd /src && go build -v -ldflags "-s -w"

# Final Stage
FROM alpine

COPY --from=build-env /src/glider /app/

RUN apk -U upgrade \
    && apk add bind-tools ca-certificates shadow \
    && groupadd -g 1000 glider \
    && useradd -r -u 1000 -g glider glider \
    && apk del shadow \
    && chown -R glider:glider /app
    && apk -v cache clean
    
WORKDIR /app
USER glider

ENTRYPOINT ["./glider"]
