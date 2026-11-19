ARG GO_IMAGE=golang:1.27-alpine
ARG RUNTIME_IMAGE=alpine:3.22
ARG GOPROXY=https://proxy.golang.org,direct
ARG GOSUMDB=sum.golang.org

FROM ${GO_IMAGE} AS build

WORKDIR /src

ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}

COPY services/api/go.mod services/api/go.sum ./
RUN go mod download

COPY services/api ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

FROM ${RUNTIME_IMAGE}

RUN apk add --no-cache ca-certificates wget tzdata \
    && adduser -D -u 10001 app

WORKDIR /app

COPY --from=build /out/api /app/api
COPY --from=build /out/migrate /app/migrate
COPY --from=build /out/seed /app/seed

RUN mkdir -p /app/var/uploads && chown -R app:app /app/var

USER app
EXPOSE 8080

ENTRYPOINT ["/app/api"]
