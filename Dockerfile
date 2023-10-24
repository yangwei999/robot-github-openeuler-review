FROM openeuler/openeuler:23.03 as BUILDER
RUN dnf update -y && \
    dnf install -y golang && \
    go env -w GOPROXY=https://goproxy.cn,direct

MAINTAINER zengchen1024<chenzeng765@gmail.com>

# build binary
WORKDIR /go/src/github.com/opensourceways/robot-github-openeuler-review
COPY . .
RUN GO111MODULE=on CGO_ENABLED=0 go build -a -o robot-github-openeuler-review .

# copy binary config and utils
FROM openeuler/openeuler:22.03
RUN dnf -y update && \
    dnf in -y shadow && \
    groupadd -g 1000 review && \
    useradd -u 1000 -g review -s /bin/bash -m review

COPY --chown=review --from=BUILDER /go/src/github.com/opensourceways/robot-github-openeuler-review/robot-github-openeuler-review /opt/app/robot-github-openeuler-review

USER review

ENTRYPOINT ["/opt/app/robot-github-openeuler-review"]
