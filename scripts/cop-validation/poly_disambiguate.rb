# Disambiguate a poly-message cop (one cop_name, several deprecations with
# different migration impact). The viable way — do NOT hand-guess message tokens
# or removal status:
#   1. read the cop's AUTHORITATIVE table from cookstyle (not RESTRICT_ON_SEND,
#      which over-approximates),
#   2. get the exact offence message cookstyle emits (the match token), and
#   3. behaviourally PROBE each construct against the live Ruby/Chef — respond_to?
#      lies (e.g. ENV.clone exists but raises), so call and catch.
# Run on a Chef Workstation host: chef exec ruby poly_disambiguate.rb
#
# Below is worked for Lint/DeprecatedClassMethods. Generalise per cop: each
# stores its table differently.
require "cookstyle"
require "socket"

k = RuboCop::Cop::Lint::DeprecatedClassMethods
puts "cop: Lint/DeprecatedClassMethods (cookstyle #{begin; Cookstyle::VERSION; rescue StandardError; '?'; end})"
puts "PREFERRED_METHODS = #{k::PREFERRED_METHODS.inspect}"
puts "receivers          = #{k::DIR_ENV_FILE_CONSTANTS.inspect}"

# NoMethodError (gone) or a runtime raise like TypeError (broken) => Blocker;
# runs without raising (maybe warns) => Review.
def verdict
  yield
  "Review (present)"
rescue NoMethodError
  "Blocker (NoMethodError — removed)"
rescue StandardError => e
  "Blocker (#{e.class} — breaks at runtime)"
end

probes = {
  "File.exists?"         => -> { File.exists?("/tmp") },
  "Dir.exists?"          => -> { Dir.exists?("/tmp") },
  "ENV.clone"            => -> { ENV.clone },
  "ENV.dup"              => -> { ENV.dup },
  "ENV.freeze"           => -> { ENV.freeze },
  "Socket.gethostbyname" => -> { Socket.gethostbyname("localhost") },
  "Socket.gethostbyaddr" => -> { Socket.gethostbyaddr([127, 0, 0, 1].pack("C4"), Socket::AF_INET) },
  "iterator?"            => -> { iterator? },
  "attr(:x, true)"       => -> { Class.new { attr :cmm_probe_x, true } },
}

puts "\nconstruct -> verdict (behaviourally probed on Ruby #{RUBY_VERSION})"
probes.each { |name, p| puts "  #{name.ljust(22)} #{verdict(&p)}" }
