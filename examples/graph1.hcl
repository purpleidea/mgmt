resource "exec" "exec1" {
  cmd = "cat /tmp/mgmt-hello-world"
  state =  "present"
}
