# Custom pod image. RunPod API v2 cannot replace a container entrypoint, and the
# public pytorch image's /start.sh ignores docker args and sleeps forever.
# Build this image, push it, and set WCKD_IMAGE (or the preset image) to it.
#
#   docker build -t ghcr.io/you/wckd-comfy-h3:dev sidecar
#
# ENTRYPOINT is the sidecar, so when the deadline hits the container drains
# and exits. RunPod then reports the pod EXITED (catalog rate drops to 0).
# `wckd stop` or the sweeper still deletes the pod record.
FROM runpod/pytorch:1.0.2-cu1281-torch280-ubuntu2404

RUN curl -fsSL https://rclone.org/install.sh | bash

COPY lib.sh bootstrap.sh drain.sh /opt/wckd/
RUN chmod 755 /opt/wckd/bootstrap.sh /opt/wckd/drain.sh

ENTRYPOINT ["/opt/wckd/bootstrap.sh"]
