FROM golang:1.20 AS build
WORKDIR /src
COPY ./ .
RUN CGO_ENABLED=0 GOOS=linux go build -o server .


FROM alpine
WORKDIR /opt/bin
COPY --from=build /src/server .
COPY ./prompts ./prompts/
COPY ./.env .
ENTRYPOINT ["./server"]
