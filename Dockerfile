# ---- build stage: compile a static binary ----
FROM golang:1.26.11-alpine3.24 AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o go-data ./cmd/server

# ---- runtime stage: just the binary + assets, no Go toolchain ----
FROM alpine:3.24

RUN apk update && apk upgrade --no-cache && apk add --no-cache dmidecode

WORKDIR /app
COPY --from=build /app/go-data ./go-data
COPY --from=build /app/web ./web
COPY --from=build /app/config.json ./config.json

EXPOSE 9000

CMD ["./go-data"]
