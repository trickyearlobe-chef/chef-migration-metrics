# Enumerate every cop in the migration-relevant departments so the false-negative
# sweep has a complete inventory to diff against the curated Blocker set.
# Run: chef exec ruby enumerate_cops.rb
# Emits TSV: DEPARTMENT<tab>COP<tab>SEVERITY<tab>RESTRICT_ON_SEND<tab>SHAPE
# SHAPE = "send" (has RESTRICT_ON_SEND methods) | "ast" (pattern/AST-shape).
require "cookstyle"
require "rubocop"

DEPARTMENTS = ["Chef/Deprecations", "Lint", "Chef/Correctness"].freeze

def restrict_on_send(k)
  k.const_defined?(:RESTRICT_ON_SEND, false) ? k.const_get(:RESTRICT_ON_SEND).to_a : []
rescue StandardError
  []
end

rows = []
RuboCop::Cop::Registry.global.cops.each do |cop|
  name = cop.cop_name # e.g. "Chef/Deprecations/NodeSet" or "Lint/DeprecatedClassMethods"
  dept = DEPARTMENTS.find { |d| name.start_with?(d + "/") }
  next unless dept

  ros = restrict_on_send(cop)
  shape = ros.empty? ? "ast" : "send"
  # default severity if the cop declares one
  sev = begin
    cfg = cop.badge # no reliable per-cop severity without config; leave blank
    ""
  rescue StandardError
    ""
  end
  rows << [dept, name, sev, ros.inspect, shape]
end

rows.sort_by! { |r| [r[0], r[1]] }
puts "DEPARTMENT\tCOP\tSEVERITY\tRESTRICT_ON_SEND\tSHAPE"
rows.each { |r| puts r.join("\t") }

puts "\n# counts by department:"
rows.group_by { |r| r[0] }.each { |d, rs| puts "#  #{d}: #{rs.size} (send=#{rs.count { |x| x[4] == 'send' }}, ast=#{rs.count { |x| x[4] == 'ast' }})" }
puts "# cookstyle #{begin; Cookstyle::VERSION; rescue StandardError; '?'; end}"
