FROM golang:1.25

WORKDIR /app
COPY go.mod ./
COPY cmd ./cmd
COPY db ./db
COPY internal ./internal

RUN go build -o /review-workflow ./cmd/server

EXPOSE 8080

CMD ["/review-workflow"]

