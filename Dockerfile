FROM golang:1.22.5 AS builder
WORKDIR /app
ADD cmd cmd
ADD internal internal
ADD go.mod go.mod
ADD go.sum go.sum
RUN go build cmd/provider/provider.go

FROM scratch AS final
COPY --from=builder /app/provider /provider
ENTRYPOINT ["/provider"]