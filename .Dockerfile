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
RUN apk add --no-cache ca-certificates
COPY --from=build-env /app /app
WORKDIR /app
ENTRYPOINT ["./glider"]