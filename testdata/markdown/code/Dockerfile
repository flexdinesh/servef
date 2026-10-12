FROM golang:1.24 AS build
WORKDIR /src
COPY . .
RUN go build -o /servef .

FROM gcr.io/distroless/base
COPY --from=build /servef /servef
ENTRYPOINT ["/servef"]
