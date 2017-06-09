resource "file" "file1" {
  path = "/tmp/mgmt-hello-world"
  content = "hello, world"
  state = "exists"
}

resource "noop" "noop1" {
  test = "nil"
}

edge "e1" {
  from = {
    kind = "noop"
    name = "noop1"
  }
  to = {
    kind = "file"
    name = "file1"
  }
}
