# Inspect the authoritative tables / messages for false-negative candidate cops so
# probes target the real construct (not RESTRICT_ON_SEND over-approximation).
# Run: chef exec ruby inspect_candidates.rb
require "cookstyle"

def cop_class(name)
  ("RuboCop::Cop::" + name.gsub("/", "::")).split("::").inject(Object) { |m, c| m.const_get(c) }
rescue StandardError
  nil
end

# Candidates whose exact target set lives in a cop constant or MSG string.
%w[
  Lint/DeprecatedConstants
  Lint/DeprecatedOpenSSLConstant
  Lint/BigDecimalNew
  Lint/ErbNewArguments
  Lint/UriEscapeUnescape
  Lint/UriRegexp
  Chef/Deprecations/UsesDeprecatedMixins
  Chef/Deprecations/UsesChefRESTHelpers
  Chef/Deprecations/ChefShellout
  Chef/Deprecations/EpicFail
  Chef/Deprecations/NodeSetWithoutLevel
  Chef/Deprecations/PartialSearchClassUsage
  Chef/Deprecations/PartialSearchHelperUsage
  Chef/Deprecations/ResourceUsesDslNameMethod
  Chef/Deprecations/ResourceUsesOnlyResourceName
].each do |name|
  k = cop_class(name)
  puts "=== #{name} #{k ? '' : '(NOT FOUND)'}"
  next unless k

  consts = k.constants(false).reject { |c| c == :RESTRICT_ON_SEND }
  consts.each do |c|
    v = k.const_get(c)
    puts "  #{c} = #{v.inspect[0, 400]}"
  end
  puts "  (no interesting constants)" if consts.empty?
end
