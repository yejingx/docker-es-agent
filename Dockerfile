FROM centos:6

RUN curl http://mirrors.aliyun.com/repo/Centos-6.repo > /etc/yum.repos.d/CentOS-Base.repo \
    && rpm -Uvh http://mirrors.aliyun.com/epel/epel-release-latest-6.noarch.rpm \
    && yum install -y golang.x86_64 git

ADD stats.go /app/stats.go

ENV GOPATH /gopath
ENV GOBIN /gopath/bin

RUN cd /app && echo $GOPATH && go get -v && go build stats.go

CMD ["/app/stats"]
