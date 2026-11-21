FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false \
    -ldflags="-s -w" \
    -o /bin/zerelio \
    ./cmd/zerelio

FROM gcr.io/distroless/static-debian12

COPY --from=builder /bin/zerelio /zerelio

EXPOSE 8080
ENTRYPOINT ["/zerelio"]
