NODE1_SSH="ssh -p 57103 -o StrictHostKeyChecking=no -o ConnectTimeout=5 kevin@127.0.0.1"
NODE2_SSH="ssh -p 57123 -o StrictHostKeyChecking=no -o ConnectTimeout=5 kevin@127.0.0.1"
KBDIAG_REMOTE="/tmp/kbdiag"

ssh_node1() {
  $NODE1_SSH "sudo -i -u kingbase bash -c $(printf '%q' "$1")" 2>/dev/null
}

ssh_node2() {
  $NODE2_SSH "sudo -i -u kingbase bash -c $(printf '%q' "$1")" 2>/dev/null
}

ssh_node1_exit() {
  local result
  result=$($NODE1_SSH "sudo -i -u kingbase bash -c $(printf '%q' "$1"); echo EXIT:\$?" 2>/dev/null)
  echo "$result" | grep -o 'EXIT:[0-9]*' | cut -d: -f2
}

ssh_node2_exit() {
  local result
  result=$($NODE2_SSH "sudo -i -u kingbase bash -c $(printf '%q' "$1"); echo EXIT:\$?" 2>/dev/null)
  echo "$result" | grep -o 'EXIT:[0-9]*' | cut -d: -f2
}

# Run ksql on node1 as kingbase
ksql_node1() {
  ssh_node1 "/home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -AXtc '$1'"
}

deploy_kbdiag() {
  local src
  src="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/dist/kbdiag"
  scp -P 57103 -q "$src" kevin@127.0.0.1:/tmp/kbdiag
  $NODE1_SSH "chmod +x /tmp/kbdiag"
  scp -P 57123 -q "$src" kevin@127.0.0.1:/tmp/kbdiag
  $NODE2_SSH "chmod +x /tmp/kbdiag"
}

setup_sql_node1() {
  local file="$1"
  scp -P 57103 -q "$file" kevin@127.0.0.1:/tmp/_kbdiag_setup.sql
  ssh_node1 "/home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -f /tmp/_kbdiag_setup.sql" || true
}
