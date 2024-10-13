FROM golang:1.23.2 AS builder
WORKDIR /app
ADD cmd cmd
ADD internal internal
ADD go.mod go.mod
ADD go.sum go.sum
RUN CGO_ENABLED=0 go build cmd/provider/provider.go

FROM scratch AS final
COPY --from=builder /app/provider /provider
ENTRYPOINT ["/provider"]
CMD ["run"]