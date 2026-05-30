FROM alpine:3.19

LABEL maintainer="ops-team"
LABEL org.opencontainers.image.title="OpsPanel"
LABEL org.opencontainers.image.description="Ops tool platform with log analysis and alerting"

RUN apk add --no-cache ca-certificates tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

COPY opspanel /usr/local/bin/opspanel
COPY templates/ /app/templates/
RUN chmod +x /usr/local/bin/opspanel

EXPOSE 9090
VOLUME /data

ENV TZ=Asia/Shanghai
ENV DATA_DIR=/data
ENV LISTEN_ADDR=:9090

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:9090/healthz || exit 1

CMD ["opspanel"]
