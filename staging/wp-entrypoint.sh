#!/usr/bin/env bash
set -euo pipefail

mkdir -p /run/sshd /etc/ssh/sshd_config.d /root/.ssh

if [[ -f /root/.ssh/authorized_keys ]]; then
  cp /root/.ssh/authorized_keys /tmp/authorized_keys
  chmod 600 /tmp/authorized_keys
fi

cat > /etc/ssh/sshd_config.d/override.conf << EOF
AuthorizedKeysFile /tmp/authorized_keys
StrictModes no
PermitUserEnvironment yes
EOF

if [[ ! -f /etc/ssh/ssh_host_rsa_key ]]; then
  ssh-keygen -A > /dev/null 2>&1
fi

/usr/sbin/sshd

set +u
. /etc/apache2/envvars
set -u
exec apache2 -DFOREGROUND
