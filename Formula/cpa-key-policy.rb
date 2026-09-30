class CpaKeyPolicy < Formula
  desc "CLIProxyAPI downstream API-key policy plugin"
  homepage "https://github.com/wignerStan/cpa-plugin-key-policy"
  license "MIT"
  head "https://github.com/wignerStan/cpa-plugin-key-policy.git", branch: "main"

  depends_on "go" => :build

  def install
    ENV["CGO_ENABLED"] = "1"
    ENV["GOFLAGS"] = "-mod=mod"
    # Some Linux Homebrew installations ship a GCC wrapper whose cc1 path is
    # incomplete. Use the system compiler for this small c-shared build; on
    # macOS /usr/bin/cc is Apple's supported clang entry point.
    ENV["CC"] = "/usr/bin/cc"
    ENV["CXX"] = "/usr/bin/c++"
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
