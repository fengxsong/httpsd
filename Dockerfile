# build
FROM golang:1.22 as builder
ARG LDFLAGS

WORKDIR /workspace
COPY go.mod go.sum /workspace/
RUN go mod download
COPY transform handler.go main.go /workspace/

RUN CGO_ENABLED=0 go build -a -ldflags "${LDFLAGS}" -o httpsd && ./httpsd --version

# run
FROM alpine:3

COPY --from=builder /workspace/httpsd /httpsd

LABEL author="fengxsong <fengxsong@outlook.com>"

EXPOSE 8080
ENTRYPOINT [ "/httpsd" ]