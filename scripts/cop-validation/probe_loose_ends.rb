# Close the two loose ends from the FP validation: DeprecatedPlatformMethods and
# the 4 non-symbol cops. Behavioural probe where a method/const can be exercised;
# flag the pure metadata-`depends` cops as needing a repo/ChefSpec harness.
# Run: chef exec ruby probe_loose_ends.rb
require "cookstyle"
require "chef"

def present_meth?(mod_path, meth)
  m = mod_path.split("::").inject(Object) { |o, c| o.const_get(c) }
  (m.respond_to?(meth) || m.methods.include?(meth) || (m.respond_to?(:instance_methods) && m.instance_methods.include?(meth)))
rescue NameError
  :no_const
end

puts "=== Chef #{Chef::VERSION} / Ruby #{RUBY_VERSION} ==="

puts "\n--- Chef/Deprecations/DeprecatedPlatformMethods (Chef::Platform.*) ---"
%i[provider_for_resource find_provider find_provider_for_node set].each do |m|
  p = present_meth?("Chef::Platform", m)
  verdict = p == true ? "PRESENT -> Review" : (p == :no_const ? "NO Chef::Platform const" : "ABSENT -> BLOCKER")
  puts "  Chef::Platform.#{m.to_s.ljust(24)} #{p.inspect.ljust(8)} #{verdict}"
end

puts "\n--- Chef/Deprecations/ChefRewind (rewind/unwind DSL) ---"
%i[rewind unwind].each do |m|
  present = Chef::DSL::Recipe.instance_methods.include?(m)
  puts "  #{m.to_s.ljust(8)} on Chef::DSL::Recipe -> #{present ? 'PRESENT -> Review' : 'ABSENT (needs chef-rewind gem) -> BLOCKER'}"
end

puts "\n--- Chef/Deprecations/LegacyNotifySyntax (Hash notify + resources() helper) ---"
puts "  resources() helper on Chef::DSL::Recipe -> " +
     (Chef::DSL::Recipe.instance_methods.include?(:resources) ? "PRESENT" : "ABSENT")
# The deprecated bit: notifies :action, <Hash> (removed in Chef 13). Probe it.
begin
  r = Chef::Resource::Execute.new("x", nil)
  r.notifies(:restart, { service: "nginx" })
  puts "  notifies(:restart, {service:'nginx'}) -> ACCEPTED -> Review"
rescue StandardError => e
  puts "  notifies(:restart, {service:'nginx'}) -> RAISES #{e.class}: #{e.message[0, 50]} -> BLOCKER"
end

puts "\n--- Chef/Deprecations/CookbookDependsOnCompatResource / CookbookDependsOnPartialSearch ---"
puts "  Pure metadata `depends 'compat_resource' | 'partial_search'` — not a client-side"
puts "  method/const removal. `depends` still resolves; whether the depended-on cookbook"
puts "  breaks a converge is a REPO/ChefSpec question, not Ruby-introspectable. -> defer to harness."
puts "  (ChefCompat::Resource const present? #{defined?(ChefCompat::Resource) ? 'yes' : 'GONE'})"
