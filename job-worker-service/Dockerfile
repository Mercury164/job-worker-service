FROM golang:1.24.1-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/app ./cmd/app
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=build /out/app /app/app
COPY --from=build /out/worker /app/worker
CMD ["/app/app"]
