class CpaKeyPolicy < Formula
  desc "CLIProxyAPI downstream API-key policy plugin"
  homepage "https://github.com/wignerStan/cpa-plugin-key-policy"
  license "MIT"
  head "https://github.com/wignerStan/cpa-plugin-key-policy.git", branch: "main"

  depends_on "go" => :build

  def install
    ENV["CGO_ENABLED"] = "1"
    ENV["GOFLAGS"] = "-mod=mod"
    extension = OS.mac? ? "dylib" : "so"
    output = libexec / "cpa-key-policy.#{extension}"
    libexec.mkpath
    system "go", "build", "-trimpath", "-buildvcs=false", "-tags", "cshared",
           "-buildmode=c-shared", "-ldflags", "-s -w", "-o", output,
           "./cmd/cpa-key-policy"
    rm libexec / "cpa-key-policy.h"
  end

  def caveats
    <<~EOS
      Set CLIProxyAPI's plugins.dir to:
        #{opt_libexec}

      Then enable cpa-key-policy and configure catalog_groups.
    EOS
  end

  test do
    extension = OS.mac? ? "dylib" : "so"
    assert_path_exists libexec / "cpa-key-policy.#{extension}"
  end
end
