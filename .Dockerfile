FROM --platform=$BUILDPLATFORM alpine as build-env
COPY ./dist /dist
RUN apk add --no-cache ca-certificates

ARG TARGETPLATFORM
RUN case $TARGETPLATFORM in \
	'linux/386') \
		export FOLDER='default_linux_386'; \
		;; \
	'linux/amd64') \
		export FOLDER='default_linux_amd64_v1'; \
		;; \
	'linux/arm/v6') \
		export FOLDER='default_linux_arm_6'; \
		;; \
	'linux/arm/v7') \
		export FOLDER='default_linux_arm_7'; \
		;; \
	'linux/arm64') \
		export FOLDER='default_linux_arm64'; \
		;; \
	'linux/riscv64') \
		export FOLDER='default_linux_riscv64'; \
		;; \
	*) echo >&2 "error: unsupported architecture '$TARGETPLATFORM'"; exit 1 ;; \
esac \
	&& mv /dist/$FOLDER /app ; \
    rm /dist -rf


FROM scratch

WORKDIR /app
COPY --from=build-env \
    /etc/ssl/certs/ca-certificates.crt \
    /etc/ssl/certs/ca-certificates.crt
COPY --from=build-env --chown=1000:1000 /app /app

USER 1000
ENTRYPOINT ["./glider"]
