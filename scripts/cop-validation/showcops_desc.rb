# Extract the --show-cops Description + any "removed in N" the curation linter
# would parse, for the false-negative candidates being added to copmapping.go.
# Run: cookstyle --show-cops > /tmp/showcops.yml; chef exec ruby showcops_desc.rb
require "yaml"
y = begin
  YAML.unsafe_load_file("/tmp/showcops.yml")
rescue NoMethodError
  YAML.load_file("/tmp/showcops.yml")
end

re = /removed in (?:chef(?: infra)?(?: client)?\s+)?(\d+)/i
%w[
  Chef/Deprecations/UsesChefRESTHelpers
  Chef/Deprecations/ChefShellout
  Chef/Deprecations/UsesDeprecatedMixins
  Chef/Deprecations/ResourceUsesDslNameMethod
  Chef/Deprecations/NodeSetWithoutLevel
  Chef/Deprecations/PartialSearchClassUsage
  Chef/Deprecations/PartialSearchHelperUsage
  Chef/Deprecations/EpicFail
  Lint/BigDecimalNew
  Lint/UnifiedInteger
  Lint/DeprecatedConstants
].each do |c|
  d = y[c]
  desc = d ? (d["Description"] || "").gsub(/\s+/, " ") : ""
  m = desc.match(re)
  puts "#{c} | present=#{d ? 'Y' : 'ABSENT'} | removed-in-desc=#{m ? m[1] : 'none'}"
  puts "    #{desc[0, 150]}"
end
