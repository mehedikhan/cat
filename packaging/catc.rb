# Homebrew formula for catc.
#
# This belongs in a tap repository, not here: create
# github.com/mehedikhan/homebrew-tap and copy this file to Formula/catc.rb.
# Users then run:
#
#   brew install mehedikhan/tap/catc
#
# Bump `version` and `sha256` on each release; the release workflow prints the
# checksums, or run `brew bump-formula-pr`.
class Catc < Formula
  desc "Ruby-flavoured language that compiles through Go"
  homepage "https://github.com/mehedikhan/cat"
  version "1.0.0"
  license "MIT"

  depends_on "go"

  on_macos do
    on_arm do
      url "https://github.com/mehedikhan/cat/releases/download/v#{version}/catc_v#{version}_darwin_arm64.tar.gz"
      sha256 "c476837389fdf392992c1c49e24c4fff988323d69940ba3590e95614b4c855d5"
    end
    on_intel do
      url "https://github.com/mehedikhan/cat/releases/download/v#{version}/catc_v#{version}_darwin_amd64.tar.gz"
      sha256 "f052230b1ab7b4f620a85313c6e03257556cd2f912ae5d7c45cefaa03008d13c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/mehedikhan/cat/releases/download/v#{version}/catc_v#{version}_linux_arm64.tar.gz"
      sha256 "3b9c61c062bb9c13bba6cce5b59a5f1b6475af5edf5706e968530f1d5b89473a"
    end
    on_intel do
      url "https://github.com/mehedikhan/cat/releases/download/v#{version}/catc_v#{version}_linux_amd64.tar.gz"
      sha256 "974e98db854b6334a543ab5129a81d29b283df627feb5777c69d72b3ecd2ad4e"
    end
  end

  def install
    bin.install "catc"
    doc.install "README.md", "HOW_IT_WORKS.md"
    pkgshare.install "examples"
  end

  def caveats
    <<~EOS
      catc compiles by generating Go source and building it, so the Go
      toolchain must be installed. Verify your setup with:
        catc doctor
    EOS
  end

  test do
    (testpath/"hello.cat").write(<<~CAT)
      puts "hello, #{"world"}!"
    CAT
    assert_match "hello, world!", shell_output("#{bin}/catc run #{testpath}/hello.cat")
  end
end
