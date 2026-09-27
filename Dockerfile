FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /botshelf ./cmd/server

FROM alpine:3.22
WORKDIR /app
RUN adduser -D -u 10001 appuser && apk add --no-cache ca-certificates
COPY --from=build /botshelf /app/botshelf
USER appuser
ENV HTTP_ADDR=:8080
EXPOSE 8080
CMD ["/app/botshelf"]
