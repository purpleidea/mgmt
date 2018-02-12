FROM centos:7
MAINTAINER Karim Boumedhel <karimboumedhel@gmail.com>

RUN yum -y install augeas-libs libvirt-libs && yum clean all
ADD mgmt /usr/bin
RUN chmod 700 /usr/bin/mgmt

ENTRYPOINT ["/usr/bin/mgmt"]
CMD ["-h"]
