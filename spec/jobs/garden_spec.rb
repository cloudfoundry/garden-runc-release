require 'rspec'
require 'json'
require 'bosh/template/test'
require 'yaml'
require 'iniparse'

describe 'garden' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('garden') }
  let(:template) { job.template('config/config.ini') }
  let(:properties) {{}}

  context 'config/config.ini' do
    context 'with defaults' do
      it 'sets the bind socket' do
        rendered_template = IniParse.parse(template.render(properties))
        expect(rendered_template['server']['bind-socket']).to eql('/var/vcap/data/garden/garden.sock')
      end
    end

    context 'with a listen address' do
      it 'throws an exception if the ip is invalid' do
        properties.merge!({
          'garden' => {
            'listen_network' => 'tcp',
            'listen_address' => '0.0.0.1:5555'
          }
        })
        
        expect {template.render(properties)}.to raise_error(IPAddr::InvalidAddressError)
      end

      it 'switches to a listen address and port' do
        properties['garden'].merge!(
          'listen_network' => 'tcp',
          'listen_address' => '127.0.0.1:5555'
        )
        
        rendered_template = IniParse.parse(template.render(properties))
        expect(rendered_template['server']['bind-ip']).to eql('127.0.0.1')
        expect(rendered_template['server']['bind-port']).to eql(5555)
      end
    end
  end
end