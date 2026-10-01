class CliproxyapiPatched < Formula
  desc "CLIProxyAPI built with the key-policy integration patches"
  homepage "https://github.com/wignerStan/cpa-plugin-key-policy"
  license "MIT"
  head "https://github.com/wignerStan/cpa-plugin-key-policy.git", branch: "main"

  depends_on "go" => :build

  conflicts_with "cliproxyapi", because: "both install a cliproxyapi binary"

  def install
    # The Linux Homebrew host may expose a broken brewed gcc wrapper whose
    # cc1 executable is missing. Use the system compiler for the CGO build.
    ENV["CC"] = "/usr/bin/cc"
    ENV["CXX"] = "/usr/bin/c++"
    system "bash", "scripts/materialize-cliproxyapi.sh"
    build_root = buildpath / "build" / "cliproxyapi"
    ldflags = %W[
      -X main.Version=#{version}
      -X main.Commit=wignerstan/cpa-plugin-key-policy
      -X main.BuildDate=#{time.iso8601}
      -X main.DefaultConfigPath=#{etc / "cliproxyapi-patched.conf"}
    ]

    Dir.chdir(build_root) do
      system "go", "build", *std_go_args(output: bin / "cliproxyapi", ldflags: ldflags),
             "./cmd/server/main.go"
      etc.install "config.example.yaml" => "cliproxyapi-patched.conf"
    end
  end

  service do
    run [opt_bin / "cliproxyapi", "-config", etc / "cliproxyapi-patched.conf"]
    keep_alive true
  end

  test do
    output = shell_output("#{bin}/cliproxyapi --version 2>&1", 2)
    assert_match "CLIProxyAPI Version:", output
  end
end
