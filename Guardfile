ignore /src/

guard :rspec, cmd: 'rspec' do
  watch(%r{^spec/.*/.+_spec\.rb$})
  watch(%r{^jobs/(.*)/.+$})     { |m| "spec/jobs/#{m[1]}_spec.rb" }
  watch('spec/spec_helper.rb')  { "spec" }
end