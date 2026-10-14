# Local LSF test image. It contains no LSF software: start-lsf.sh installs LSF
# from the packages mounted at /lsf-distrib when the container starts.
FROM rockylinux:8

ARG TEST_UID
ARG TEST_GID

RUN dnf install -y --setopt=install_weak_deps=False \
        ed file findutils gzip hostname libnsl net-tools procps-ng tar which \
    && dnf clean all

# The test user shares the host user's IDs so the host can remove /state.
RUN groupadd -o -g "$TEST_GID" rotari \
    && useradd -o -m -u "$TEST_UID" -g "$TEST_GID" rotari \
    && useradd -m lsfadmin

COPY start-lsf.sh /usr/local/bin/start-lsf.sh
ENTRYPOINT ["/usr/local/bin/start-lsf.sh"]
