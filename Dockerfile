# Multi-stage build compatible with linux/arm64 and linux/amd64.
FROM golang:1.22-alpine AS build
WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=build /out/server /app/server
COPY internal/httpapi/assets /app/internal/httpapi/assets

ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/server"]
