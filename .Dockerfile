FROM alpine AS build-env
COPY ./dist /dist
RUN arch="$(apk --print-arch)"; \
	case "$arch" in \
		'x86_64') \
			export FOLDER='default_linux_amd64'; \
			;; \
		'armhf') \
			export FOLDER='default_linux_arm_6'; \
			;; \
		'armv7') \
			export FOLDER='default_linux_arm_7'; \
			;; \
		'aarch64') \
			export FOLDER='default_linux_arm64'; \
			;; \
		'x86') \
			export FOLDER='default_linux_386'; \
			;; \
		*) echo >&2 "error: unsupported architecture '$arch'"; exit 1 ;; \
	esac \
    && mv /dist/$FOLDER /app ; \
    rm /dist -rf

FROM alpine

WORKDIR /app
COPY --from=build-env /app /app

RUN apk -U upgrade --no-cache \
    && apk --no-cache add ca-certificates shadow \
    && groupadd -g 1000 glider \
    && useradd -r -u 1000 -g glider glider \
    && apk --no-cache del shadow \
    && chown -R glider:glider /app \
    && chmod +x /app/glider
    
USER glider
ENTRYPOINT ["./glider"]
