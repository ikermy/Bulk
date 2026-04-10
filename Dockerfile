FROM golang:1.25-bookworm as builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/bulk ./cmd/server

FROM gcr.io/distroless/static-debian11
COPY --from=builder /app/bulk /bulk
EXPOSE 8080
ENTRYPOINT ["/bulk"]

