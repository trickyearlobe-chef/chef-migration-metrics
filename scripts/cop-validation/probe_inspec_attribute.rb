# Probe the InSpec/Deprecations/Attribute* cops. Are `attribute`/`attributes`
# REMOVED in the target InSpec, or just a deprecated alias of `input`?
# Introspect the cop MSG + behaviourally check the DSL.
# Run: chef exec ruby probe_inspec_attribute.rb
require "cookstyle"

def cop_class(name)
  ("RuboCop::Cop::" + name.gsub("/", "::")).split("::").inject(Object) { |m, c| m.const_get(c) }
rescue StandardError
  nil
end

%w[InSpec/Deprecations/AttributeHelper InSpec/Deprecations/AttributeDefault].each do |n|
  k = cop_class(n)
  puts "=== #{n} #{k ? '' : '(NOT in cookstyle registry)'}"
  next unless k
  k.constants(false).each do |c|
    v = k.const_get(c)
    puts "  #{c} = #{v.inspect[0, 300]}"
  end
end

puts "\n--- InSpec runtime: is `attribute` removed or still an alias of `input`? ---"
begin
  require "inspec"
  puts "  InSpec::VERSION = #{Inspec::VERSION rescue 'unknown'}"
  # The DSL method lives on the profile context / Inspec::Input machinery.
  ctx_mod = begin
    Inspec::Resources # force load
    nil
  rescue StandardError
    nil
  end
  # Check the profile DSL for :attribute vs :input.
  dsl = Inspec.const_defined?(:DSL) ? Inspec::DSL : nil
  probe_targets = []
  probe_targets << ["Inspec::DSL#attribute", dsl && dsl.instance_methods.include?(:attribute)]
  probe_targets << ["Inspec::DSL#input", dsl && dsl.instance_methods.include?(:input)]
  # Also the input registry
  if Inspec.const_defined?(:InputRegistry)
    puts "  Inspec::InputRegistry present"
  end
  probe_targets.each { |label, present| puts "  #{label.ljust(28)} -> #{present ? 'PRESENT' : 'absent/na'}" }
rescue LoadError => e
  puts "  (could not load inspec: #{e.message[0, 60]})"
end
