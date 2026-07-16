# False-negative sweep: behaviourally probe every not-yet-curated cop whose target
# could be genuinely removed/broken on CC19 (Chef 19.3.15 / Ruby 3.4.8). A probe
# that raises NoMethodError/NameError (gone) or TypeError/ArgumentError (breaks at
# runtime) => hidden-Blocker candidate; a probe that runs (maybe warns) => Review.
# respond_to? lies, so CALL and catch. Run: chef exec ruby probe_candidates.rb
require "cookstyle" # workstation copy first (introspection); chef pulls vendored 8.6.10 next
require "chef"
require "uri"
require "erb"
require "openssl"
begin; require "bigdecimal"; rescue LoadError; end

NODE = Chef::Node.new

# verdict(&blk): classify a construct by how the live runtime reacts to CALLING it.
def verdict
  yield
  "PRESENT (runs) -> Review"
rescue NoMethodError => e
  "REMOVED (NoMethodError) -> BLOCKER  [#{e.message[0, 60]}]"
rescue NameError => e
  "REMOVED (#{e.class}: #{e.message[0, 40]}) -> BLOCKER"
rescue ArgumentError => e
  "BREAKS (ArgumentError: #{e.message[0, 50]}) -> BLOCKER"
rescue TypeError => e
  "BREAKS (TypeError: #{e.message[0, 50]}) -> BLOCKER"
rescue StandardError => e
  "RAISES (#{e.class}: #{e.message[0, 50]}) -> BLOCKER?"
end

# const_present?: is a Ruby/Chef constant still defined? (for removed-class cops)
def const_present?(path)
  path.split("::").inject(Object) { |m, c| m.const_get(c) }
  "PRESENT (const defined) -> Review"
rescue NameError
  "REMOVED (const gone) -> BLOCKER"
end

puts "=== Chef #{Chef::VERSION} / Ruby #{RUBY_VERSION} ==="
puts "\n--- Ruby-API removal cops (Lint dept, run on the bundled Ruby) ---"

ruby_probes = {
  "Lint/BigDecimalNew            (BigDecimal.new)"        => -> { BigDecimal.new("1") },
  "Lint/UriEscapeUnescape        (URI.escape)"            => -> { URI.escape("a b") },
  "Lint/UriEscapeUnescape        (URI.unescape)"          => -> { URI.unescape("a%20b") },
  "Lint/UriEscapeUnescape        (URI.encode)"            => -> { URI.encode("a b") },
  "Lint/UriEscapeUnescape        (URI.decode)"            => -> { URI.decode("a%20b") },
  "Lint/UriRegexp                (URI.regexp)"            => -> { URI.regexp },
  "Lint/ErbNewArguments          (ERB.new safe_level)"    => -> { ERB.new("x", nil, "-") },
  "Lint/DeprecatedOpenSSLConstant(OpenSSL::Cipher::Cipher)" => -> { OpenSSL::Cipher::Cipher.new("aes-128-cbc") },
  "Lint/DeprecatedOpenSSLConstant(OpenSSL::Digest::Digest)" => -> { OpenSSL::Digest::Digest.new("SHA256") },
}
ruby_probes.each { |name, p| puts "  #{name.ljust(46)} #{verdict(&p)}" }

puts "\n--- Lint/DeprecatedConstants (probe each removed constant directly) ---"
%w[NIL TRUE FALSE Fixnum Bignum Data].each do |c|
  # These are top-level; Data is NEW in 3.2 (present), the rest removed in Ruby 3.
  puts "  ::#{c.ljust(10)} #{const_present?(c)}"
end
{
  "Random::DEFAULT"          => "Random::DEFAULT",
  "Net::HTTPServerException" => "Net::HTTPServerException",
}.each do |label, path|
  begin; require "net/http"; rescue LoadError; end
  puts "  #{label.ljust(26)} #{const_present?(path)}"
end

puts "\n--- Chef removed-class / removed-helper cops (Chef/Deprecations dept) ---"
puts "  Chef/Deprecations/UsesChefRESTHelpers   Chef::REST         -> #{const_present?('Chef::REST')}"
puts "  Chef/Deprecations/ChefShellout          Chef::ShellOut     -> #{const_present?('Chef::ShellOut')}"
puts "  Chef/Deprecations/PartialSearchClassUsage Chef::PartialSearch -> #{const_present?('Chef::PartialSearch')}"

# Removed mixins (UsesDeprecatedMixins): cop flags include/require of these.
%w[
  Chef::Mixin::LanguageIncludeRecipe
  Chef::Mixin::Language
  Chef::Mixin::RecipeDefinitionDSLCore
  Chef::Mixin::Command
].each do |m|
  puts "  Chef/Deprecations/UsesDeprecatedMixins  #{m.split('::').last.ljust(28)} -> #{const_present?(m)}"
end

puts "\n--- Chef DSL/method removals (behavioural) ---"
# partial_search helper method
puts "  Chef/Deprecations/PartialSearchHelperUsage partial_search -> " +
     (Chef::DSL::Recipe.instance_methods.include?(:partial_search) ? "PRESENT -> Review" : "REMOVED -> BLOCKER")
# dsl_name class method on Chef::Resource
puts "  Chef/Deprecations/ResourceUsesDslNameMethod Chef::Resource.dsl_name -> " +
     (Chef::Resource.respond_to?(:dsl_name) ? "PRESENT -> Review" : "REMOVED -> BLOCKER")
# epic_fail
puts "  Chef/Deprecations/EpicFail  epic_fail (recipe DSL) -> " +
     (Chef::DSL::Recipe.instance_methods.include?(:epic_fail) ? "PRESENT -> Review" : "REMOVED -> BLOCKER")

# NodeSetWithoutLevel: node['x'] = y without precedence level
puts "  Chef/Deprecations/NodeSetWithoutLevel  node['x']=y -> " + verdict { NODE["cmm_probe_zzz"] = 1 }
puts "  Chef/Deprecations/NodeSetWithoutLevel  node['x']<<y -> " + verdict { (NODE["cmm_probe_arr"] ||= []) << 1 }

# ResourceUsesOnlyResourceName: resource_name without provides -> build-time failure on CC16+
begin
  r = Class.new(Chef::Resource) do
    resource_name :cmm_probe_only_resource_name
  end
  inst = r.new("x", nil)
  puts "  Chef/Deprecations/ResourceUsesOnlyResourceName resource_name-only builds -> PRESENT (built: #{inst.class.resource_name}) -> Review"
rescue StandardError => e
  puts "  Chef/Deprecations/ResourceUsesOnlyResourceName resource_name-only -> RAISES (#{e.class}) -> BLOCKER?"
end
