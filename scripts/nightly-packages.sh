#!/usr/bin/env bash
# 原生包打包脚本：把一个已编译好的 linux 二进制打成 deb + rpm。
#
# 选型说明：nfpm 需要额外安装；dpkg-deb 与 rpmbuild 在 ubuntu 镜像上原生可用，
# 走脚本更稳，也便于 CI 首跑前在本机先验。配置目录与 systemd unit 一并打进去，
# 装完即可 `systemctl start <name>`（post/preun 钩子做 reload/start/stop）。
#
# 用法：nightly-packages.sh <name> <binary> <staging> <os-arch> <out-dir> <base-version> <build-tag>
#   name         包名，如 spawn-gateway
#   binary       已编译的 linux 二进制（CGO_ENABLED=0 交叉编译产物）
#   staging      配置文件目录（等值复制进 /etc/<name>/）
#   os-arch      linux-amd64 | linux-arm64
#   out-dir      产物输出目录
#   base-version 数字开头的基线版本（deb/rpm 都要求），nightly 固定 0.0.0
#   build-tag    构建标记（如 nightly20261114.abc1234），deb 走 ~ 后缀、
#                rpm 走 Release，均不构成正式版本号（铁律：不自行发版）
set -euo pipefail

NAME="$1"; BIN="$2"; STAGE="$3"; OSARCH="$4"; OUT="$5"; BASE_VERSION="$6"; BUILD_TAG="$7"

# Go arch → dpkg/rpm arch：amd64→amd64/x86_64，arm64→arm64/aarch64
case "$OSARCH" in
linux-amd64) DEB_ARCH=amd64; RPM_ARCH=x86_64 ;;
linux-arm64) DEB_ARCH=arm64;  RPM_ARCH=aarch64 ;;
*) echo "unsupported os-arch: $OSARCH" >&2; exit 2 ;;
esac

[ -f "$BIN" ] || { echo "binary not found: $BIN" >&2; exit 2; }
mkdir -p "$OUT"

UNIT="/usr/lib/systemd/system/$NAME.service"
CONF=/etc/$NAME/gateway-api.yaml

# ---- deb -----------------------------------------------------------------
DEB_DIR="$(mktemp -d)"
mkdir -p "$DEB_DIR/usr/local/bin" "$DEB_DIR/etc/$NAME" \
  "$DEB_DIR/usr/lib/systemd/system" "$DEB_DIR/var/log/$NAME"
install -m 0755 "$BIN" "$DEB_DIR/usr/local/bin/$NAME"
if [ -d "$STAGE" ] && [ -n "$(ls -A "$STAGE" 2>/dev/null || true)" ]; then
  cp -a "$STAGE"/. "$DEB_DIR/etc/$NAME/"
fi

cat > "$DEB_DIR$UNIT" <<'UNIT'
[Unit]
Description=%N% (nightly build)
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/%N% -f /etc/%N%/gateway-api.yaml
Restart=on-failure
RestartSec=3
WorkingDirectory=/

[Install]
WantedBy=multi-user.target
UNIT
sed -i "s|%N%|$NAME|g" "$DEB_DIR$UNIT"

mkdir -p "$DEB_DIR/DEBIAN"
INSTALLED_SIZE=$(du -sk "$DEB_DIR" | cut -f1)
{
  echo "Package: $NAME"
  echo "Version: ${BASE_VERSION}~${BUILD_TAG}"
  echo "Section: web"
  echo "Priority: optional"
  echo "Architecture: $DEB_ARCH"
  echo "Maintainer: cuihairu <chuihairu@gmail.com>"
  echo "Installed-Size: $INSTALLED_SIZE"
  echo "Description: $NAME nightly build"
  echo " Rolling nightly package of the $NAME service."
} > "$DEB_DIR/DEBIAN/control"

# postinst/preinst 放 DEBIAN/（控制文件与钩子都属 DEBIAN/，包体不放钩子）
cat > "$DEB_DIR/DEBIAN/postinst" <<POST
#!/bin/sh
if [ -x /usr/sbin/service ] && [ \$1 -eq 1 ]; then
  /usr/sbin/service $NAME start >/dev/null 2>&1 || true
  /usr/sbin/service $NAME stop >/dev/null 2>&1 || true
fi
POST
cat > "$DEB_DIR/DEBIAN/prerm" <<PRERM
#!/bin/sh
if [ \$1 = remove ] || [ \$1 = deconfigure ]; then
  /usr/sbin/service $NAME stop >/dev/null 2>&1 || true
fi
PRERM
chmod 0755 "$DEB_DIR/DEBIAN/postinst" "$DEB_DIR/DEBIAN/prerm"

dpkg-deb --build -Zgzip "$DEB_DIR" "$OUT/${NAME}-nightly-${OSARCH}.deb"
rm -rf "$DEB_DIR"

# ---- rpm -----------------------------------------------------------------
TOP="$(mktemp -d)"; mkdir -p "$TOP"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
BUILD="$TOP/BUILD/$NAME"
mkdir -p "$BUILD/usr/local/bin" "$BUILD/usr/lib/systemd/system" \
  "$BUILD/etc/$NAME" "$BUILD/var/log/$NAME"
install -m 0755 "$BIN" "$BUILD/usr/local/bin/$NAME"
if [ -d "$STAGE" ] && [ -n "$(ls -A "$STAGE" 2>/dev/null || true)" ]; then
  cp -a "$STAGE"/. "$BUILD/etc/$NAME/"
fi
cat > "$BUILD$UNIT" <<'UNIT'
[Unit]
Description=%N% (nightly build)
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/%N% -f /etc/%N%/gateway-api.yaml
Restart=on-failure
RestartSec=3
WorkingDirectory=/

[Install]
WantedBy=multi-user.target
UNIT
sed -i "s|%N%|$NAME|g" "$BUILD$UNIT"

cat > "$TOP/SPECS/$NAME.spec" <<SPEC
Name: $NAME
Version: $BASE_VERSION
Release: $BUILD_TAG
Summary: $NAME nightly build
License: MIT
URL: https://github.com/cuihairu/spawn
Group: System Environment/Daemons
AutoReqProv: no

%description
Rolling nightly package of the $NAME service.

%install
cp -a $BUILD/. %{buildroot}/

%files
/usr/local/bin/$NAME
/etc/$NAME/
/usr/lib/systemd/system/$NAME.service
%dir /var/log/$NAME

%post
if [ \$1 -eq 1 ]; then
  /usr/bin/systemctl daemon-reload >/dev/null 2>&1 || true
  /usr/bin/systemctl start $NAME >/dev/null 2>&1 || true
fi

%preun
if [ \$1 -eq 0 ]; then
  /usr/bin/systemctl stop $NAME >/dev/null 2>&1 || true
  /usr/bin/systemctl daemon-reload >/dev/null 2>&1 || true
fi
SPEC

# 必须传 --target：spec 无合法 Arch tag（rpm 会报 Unknown tag），不传则 rpmbuild
# 用构建机架构出包，arm64 包会被打成 x86_64——载荷虽是真 aarch64，元数据却是 x86_64，
# arm64 主机 rpm -i 会因架构不符拒装（G2-29）。
rpmbuild -bb --define "_topdir $TOP" --target "$RPM_ARCH" "$TOP/SPECS/$NAME.spec" >/dev/null
# 传了 --target 后产物文件名确定，按扩展名取（同一 spec 只会出一个二进制包）
find "$TOP/RPMS" -name "*.rpm" -exec mv {} "$OUT/${NAME}-nightly-${OSARCH}.rpm" \;
# 架构断言：打包产出后立即核对元数据。载荷对了而元数据错（--target 漏传时）
# 这类缺陷，下游要装到 arm64 主机上才会暴露，必须在这里当场红。
rpm -qp --queryformat '%{ARCH}\n' "$OUT/${NAME}-nightly-${OSARCH}.rpm" >/tmp/rpmarch.$$
got=$(cat /tmp/rpmarch.$$)
echo "rpm arch: ${got} (期望 ${RPM_ARCH})"
[ "$got" = "$RPM_ARCH" ] || { echo "rpm 架构不符，拒绝产出" >&2; rm -f "$OUT/${NAME}-nightly-${OSARCH}.rpm" /tmp/rpmarch.$$; exit 3; }
rm -f /tmp/rpmarch.$$
rm -rf "$TOP"

ls -lh "$OUT" | tail -5
