resource "file" "file1" {
  path = "/tmp/mgmt-hello-world"
  content = "hello, world"
  state = "exists"
  depends_on = ["noop.noop1", "exec.sleep"]
}

resource "noop" "noop1" {
  test = "nil"
}

resource "exec" "sleep" {
 cmd = "sleep 10s"
}
