# syntax=docker/dockerfile:1
FROM golang:1.27 AS build
WORKDIR /src
COPY . .
RUN go build -o /out/app ./cmd/app && \
    echo "built"
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/out/app", "--port", "$PORT"]
