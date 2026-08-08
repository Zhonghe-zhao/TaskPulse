FROM golang:1.24 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG APP=taskpulse
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${APP}

FROM alpine:3.22

RUN apk add --no-cache ca-certificates wget
COPY --from=builder /out/app /usr/local/bin/taskpulse-app

USER 65532:65532
ENTRYPOINT ["/usr/local/bin/taskpulse-app"]
