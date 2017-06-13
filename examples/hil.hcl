resource "file" "file1" {
  path = "/tmp/mgmt-hello-world"
  content = "${exec.sleep.Output}"
  state = "exists"
}

resource "exec" "sleep" {
 cmd = "echo hello"
}
