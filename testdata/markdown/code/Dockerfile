FROM golang:1.24 AS build
WORKDIR /src
COPY . .
RUN go build -o /srcz .

FROM gcr.io/distroless/base
COPY --from=build /srcz /srcz
ENTRYPOINT ["/srcz"]
