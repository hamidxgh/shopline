FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/shopline ./cmd/shopline
RUN CGO_ENABLED=0 go build -o /shopline ./cmd/shopline

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /shopline /shopline
EXPOSE 8080
ENTRYPOINT ["/shopline"]
