# Homebrew Formula for Terra-Prism
# To use this formula, create a tap repository:
#   1. Create repo: github.com/CaptShanks/homebrew-tap
#   2. Add this file as Formula/terraprism.rb
#   3. Users install with: brew tap CaptShanks/tap && brew install terraprism

class Terraprism < Formula
  desc "Interactive terminal UI for viewing Terraform/OpenTofu plans"
  homepage "https://github.com/CaptShanks/terraprism"
  version "0.1.0"
  license "MIT"

  on_macos do
    on_intel do
      url "https://github.com/CaptShanks/terraprism/releases/download/v#{version}/terraprism-darwin-amd64"
      sha256 "REPLACE_WITH_SHA256_HASH"
    end
    on_arm do
      url "https://github.com/CaptShanks/terraprism/releases/download/v#{version}/terraprism-darwin-arm64"
      sha256 "REPLACE_WITH_SHA256_HASH"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/CaptShanks/terraprism/releases/download/v#{version}/terraprism-linux-amd64"
      sha256 "REPLACE_WITH_SHA256_HASH"
    end
    on_arm do
      url "https://github.com/CaptShanks/terraprism/releases/download/v#{version}/terraprism-linux-arm64"
      sha256 "REPLACE_WITH_SHA256_HASH"
    end
  end

  def install
    binary_name = "terraprism"
    if OS.mac?
      if Hardware::CPU.arm?
        binary_name = "terraprism-darwin-arm64"
      else
        binary_name = "terraprism-darwin-amd64"
      end
    elsif OS.linux?
      if Hardware::CPU.arm?
        binary_name = "terraprism-linux-arm64"
      else
        binary_name = "terraprism-linux-amd64"
      end
    end
    
    bin.install binary_name => "terraprism"
  end

  test do
    assert_match "terraprism", shell_output("#{bin}/terraprism --version")
  end
end

