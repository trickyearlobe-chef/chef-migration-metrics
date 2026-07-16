# Self-validating Blocker harness (spike v1).
# Run with: chef exec ruby cop_validator.rb
# Introspects each curated-Blocker cop from cookstyle, probes its target against
# the live Chef Infra Client, and reconciles with our curated RemovedIn.
require "cookstyle" # load workstation cookstyle (8.7.x) BEFORE chef pulls its vendored copy
require "chef"
require "socket"

# --- Curated Blocker set (cop_name => curated RemovedIn from copmapping.go) ------
CURATED = {
  "Chef/Deprecations/NodeSet" => "14.0",
  "Chef/Deprecations/NodeSetUnless" => "14.0",
  "Chef/Deprecations/NodeMethodsInsteadofAttributes" => "16.0",
  "Chef/Deprecations/DeprecatedShelloutMethods" => "15.0",
  "Chef/Deprecations/UsesRunCommandHelper" => "13.0",
  "Chef/Deprecations/ResourceInheritsFromCompatResource" => "14.0",
  "Chef/Deprecations/ResourceUsesProviderBaseMethod" => "14.0",
  "Chef/Deprecations/EasyInstallResource" => "13.0",
  "Chef/Deprecations/ErlCallResource" => "13.0",
  "Chef/Deprecations/UserDeprecatedSupportsProperty" => "13.0",
  "Chef/Deprecations/RequireRecipe" => "14.0",
  "Chef/Deprecations/ResourceUsesUpdatedMethod" => "13.0",
  "Chef/Deprecations/EOLAuditModeUsage" => "15.0",
  "Chef/Deprecations/ChefHandlerUsesSupports" => "14.0",
  "Chef/Deprecations/RubyBlockCreateAction" => "14.0",
  "Chef/Deprecations/DeprecatedYumRepositoryActions" => "14.0",
  "Chef/Deprecations/WindowsTaskChangeAction" => "14.0",
  "Chef/Deprecations/WindowsFeatureServermanagercmd" => "15.0",
  "Chef/Deprecations/UseInlineResourcesDefined" => "14.0",
  "Chef/Deprecations/SearchUsesPositionalParameters" => "14.0",
  "Chef/Deprecations/DeprecatedPlatformMethods" => "15.0",
  "Chef/Deprecations/ChefRewind" => "14.0",
  "Chef/Deprecations/CookbookDependsOnCompatResource" => "14.0",
  "Chef/Deprecations/CookbookDependsOnPartialSearch" => "14.0",
  "Lint/DeprecatedClassMethods" => "19.0",
}

# --- Probe strategies: (cop => how to check its target is present on this Chef) --
# Returns true=present (works), false=removed, :na=not symbol-probeable here.
NODE = Chef::Node.new
def const?(path); path.split("::").inject(Object) { |m, c| m.const_get(c) }; true; rescue StandardError; false; end
def klass(path); path.split("::").inject(Object) { |m, c| m.const_get(c) }; rescue StandardError; nil; end

PROBES = {
  # Behavioural probe (call + catch): respond_to? gives false positives for node
  # attr methods (the "ghost respond_to"), so actually invoke and watch for NoMethodError.
  "Chef/Deprecations/NodeSet" => -> { begin; NODE.set["cmm"] = 1; true; rescue NoMethodError; false; rescue StandardError; true; end },
  "Chef/Deprecations/NodeSetUnless" => -> { begin; NODE.set_unless["cmm"] = 1; true; rescue NoMethodError; false; rescue StandardError; true; end },
  "Chef/Deprecations/NodeMethodsInsteadofAttributes" => lambda {
    begin; NODE.__cmm_absent_attr_zzz; true; rescue NoMethodError; false; rescue StandardError; true; end
  },
  "Chef/Deprecations/DeprecatedShelloutMethods" => lambda {
    o = Object.new; o.extend(Chef::Mixin::ShellOut); o.respond_to?(:shell_out_compact)
  },
  "Chef/Deprecations/UsesRunCommandHelper" => -> { const?("Chef::Mixin::Command") },
  "Chef/Deprecations/ResourceInheritsFromCompatResource" => -> { const?("ChefCompat::Resource") },
  "Chef/Deprecations/ResourceUsesProviderBaseMethod" => -> { const?("Chef::Provider::LWRPBase") },
  "Chef/Deprecations/EasyInstallResource" => -> { Chef::DSL::Resources.instance_methods.include?(:easy_install) },
  "Chef/Deprecations/ErlCallResource" => -> { Chef::DSL::Resources.instance_methods.include?(:erl_call) },
  "Chef/Deprecations/UserDeprecatedSupportsProperty" => -> { Chef::Resource::User.properties.key?(:supports) },
  "Chef/Deprecations/RequireRecipe" => -> { Chef::DSL::Recipe.instance_methods.include?(:require_recipe) },
  "Chef/Deprecations/ResourceUsesUpdatedMethod" => -> { Chef::Provider.instance_methods.include?(:updated=) },
  "Chef/Deprecations/EOLAuditModeUsage" => -> { const?("Chef::Audit") },
  "Chef/Deprecations/ChefHandlerUsesSupports" => -> { const?("Chef::Handler") && Chef::Handler.instance_methods.include?(:supports) },
  "Chef/Deprecations/RubyBlockCreateAction" => -> { Chef::Resource::RubyBlock.allowed_actions.include?(:create) },
  "Chef/Deprecations/DeprecatedYumRepositoryActions" => lambda {
    k = klass("Chef::Resource::YumRepository"); k ? k.allowed_actions.include?(:add) : :na
  },
  "Chef/Deprecations/WindowsTaskChangeAction" => lambda {
    k = klass("Chef::Resource::WindowsTask"); k ? k.allowed_actions.include?(:change) : :na
  },
  "Chef/Deprecations/WindowsFeatureServermanagercmd" => lambda {
    k = klass("Chef::Resource::WindowsFeature"); next :na unless k
    begin; k.new("x").install_method(:servermanagercmd); true; rescue StandardError; false; end
  },
  "Chef/Deprecations/UseInlineResourcesDefined" => -> { Chef::Provider.respond_to?(:use_inline_resources) },
  "Chef/Deprecations/SearchUsesPositionalParameters" => lambda {
    Chef::Search::Query.instance_method(:search).parameters.any? { |t, _| t == :rest || t == :opt }
  },
  "Chef/Deprecations/DeprecatedPlatformMethods" => :na, # resolved via introspection below
  "Chef/Deprecations/ChefRewind" => :na,
  "Chef/Deprecations/CookbookDependsOnCompatResource" => -> { const?("ChefCompat::Resource") },
  "Chef/Deprecations/CookbookDependsOnPartialSearch" => :na,
  "Lint/DeprecatedClassMethods" => :poly, # handled specially below
}

def cop_class(name)
  ("RuboCop::Cop::" + name.gsub("/", "::")).split("::").inject(Object) { |m, c| m.const_get(c) }
rescue StandardError
  nil
end

def restrict_on_send(k)
  k && k.const_defined?(:RESTRICT_ON_SEND) ? k.const_get(:RESTRICT_ON_SEND).to_a : []
end

def blocker?(removed_in, target = "19.3.15")
  a = removed_in.split(".").map(&:to_i); b = target.split(".").map(&:to_i)
  (a <=> b) <= 0
end

def verdict(curated_removed, present)
  return "NA (not symbol-probeable)" if present == :na
  cur_block = blocker?(curated_removed)
  if present == false
    cur_block ? "CONFIRMED blocker" : "curated-not-blocker but REMOVED (false-negative?)"
  else # present == true
    cur_block ? "OVER-CLAIM (still works)" : "ok (present, not curated-blocker)"
  end
end

puts "=== Chef #{Chef::VERSION} / Ruby #{RUBY_VERSION} / Cookstyle #{Cookstyle::VERSION rescue 'loaded'} ==="
puts "COP|CURATED_REMOVEDIN|RESTRICT_ON_SEND|PRESENT|VERDICT"

CURATED.keys.each do |cop|
  k = cop_class(cop)
  ros = restrict_on_send(k).inspect
  probe = PROBES[cop]
  if probe == :poly
    # Auto-derive DeprecatedClassMethods variants from the cop itself, probe each.
    present = "see POLY lines"
    puts "#{cop}|#{CURATED[cop]}|#{ros}|#{present}|AUTO-DERIVED (below)"
    next
  end
  present = probe == :na ? :na : (probe.call rescue "ERR")
  puts "#{cop}|#{CURATED[cop]}|#{ros}|#{present}|#{verdict(CURATED[cop], present)}"
end

# --- Poly cop auto-derivation: probe each Ruby method the cop actually flags -----
puts "\n=== POLY: Lint/DeprecatedClassMethods per-method (auto-probed vs real Ruby) ==="
dcm = cop_class("Lint/DeprecatedClassMethods")
# class-methods: probe the class object's respond_to?; others are Kernel/Module methods.
class_methods = { exists?: [File, Dir], gethostbyname: [Socket], gethostbyaddr: [Socket] }
restrict_on_send(dcm).each do |m|
  present =
    if class_methods[m]
      class_methods[m].any? { |r| r.respond_to?(m) }
    else
      (Object.new.respond_to?(m, true) || Module.private_method_defined?(m) || Object.private_method_defined?(m)) rescue false
    end
  where = (class_methods[m] || [:Kernel_or_Module]).map(&:to_s).join("/")
  puts "  #{m} (#{where}) -> #{present ? 'PRESENT -> Review' : 'REMOVED -> Blocker'}"
end

# --- Resolve the previously-inconclusive DeprecatedPlatformMethods --------------
puts "\n=== DeprecatedPlatformMethods: what does it actually match? ==="
dpm = cop_class("Chef/Deprecations/DeprecatedPlatformMethods")
puts "  RESTRICT_ON_SEND=#{restrict_on_send(dpm).inspect}"
puts "  (source: #{dpm ? dpm.instance_method(:on_send).source_location.inspect : 'n/a'})" rescue nil
