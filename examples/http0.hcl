resource "http" "get_config" {
  url = "https://raw.githubusercontent.com/purpleidea/mgmt/master/examples/graph0.hcl"
}

resource "file" "graph" {
  path = "/tmp/mgmt-hello-world"
  content = "${http.get_config.Response}"
  state = "exists"
  depends_on = ["http.get_config"]
}
