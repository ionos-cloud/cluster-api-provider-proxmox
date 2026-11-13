# IONOS: the NIC holding the default route is public (DHCP); the other one joins the private LAN.
physical_nics() {
  for d in /sys/class/net/*/device; do basename "$(dirname "$d")"; done
}

nics_ready() {
  [ "$(physical_nics | wc -l)" -ge 2 ] && ip -o route get 1.1.1.1 >/dev/null 2>&1
}

network_detect() {
  retry nics_ready
  ip -o route get 1.1.1.1 | sed -n 's/.* dev \([^ ]*\).*/\1/p' >"$STATE/public_if"
  physical_nics | grep -vxF "$(cat "$STATE/public_if")" | head -n1 >"$STATE/private_if"
}

network_interfaces() {
  public_if=$(cat "$STATE/public_if")
  private_if=$(cat "$STATE/private_if")
  cat <<EOF
auto $public_if
iface $public_if inet dhcp

auto $private_if
iface $private_if inet static
	address $PRIVATE_IP/$PRIVATE_PREFIX
EOF
}
