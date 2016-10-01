module.exports = {
  entry: "./mgmt.js",
  devtool: "source-map",
  output: {
    path: __dirname + "/dist",
    filename: "bundlemgmt.js",
    sourceMapFilename: "[file].map",
  },
  externals: {
    "jquery": "jQuery",
    "cockpit": "cockpit",
  }
}
