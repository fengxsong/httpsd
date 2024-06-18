# build
FROM golang:1.22 as builder
ARG LDFLAGS

WORKDIR /workspace

COPY . /workspace/

RUN CGO_ENABLED=0 go build -a -ldflags "${LDFLAGS}" -o httpsd && ./httpsd --version

# run
FROM alpine:3

COPY --from=builder /workspace/httpsd /httpsd

LABEL author="fengxsong <fengxsong@outlook.com>"

EXPOSE 8080
ENTRYPOINT [ "/httpsd" ]