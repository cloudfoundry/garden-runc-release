require 'rspec'
require 'json'
require 'bosh/template/test'
require 'yaml'
require 'iniparse'

describe 'garden' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('garden') }
  let(:template) { job.template('config/config.ini') }
  let(:properties) {
    {
      'garden' => {
        'listen_network' => 'unix',
        'listen_address' => '/var/vcap/data/garden/garden.sock',
        'network_mtu' => 0,
        'deny_networks' => [],
        'allow_host_access' => false,
        'docker_registry_endpoint' => nil,
        'log_level' => 'info',
        'debug_listen_address' => nil,
        'default_container_grace_time' => 0,
        'default_container_blockio_weight' => 0,
        'default_container_rootfs' => '/var/vcap/packages/busybox/busybox-1.27.2.tar',
        'graph_cleanup_threshold_in_mb' => -1,
        'port_pool.start' => nil,
        'port_pool.size' => nil,
        'dropsonde.origin' => nil,
        'dropsonde.destination' => nil,
        'dns_servers' => [],
        'additional_dns_servers' => [],
        'additional_host_entries' => [],
        'insecure_docker_registry_list' => [],
        'runtime_plugin' => '/var/vcap/packages/runc/bin/runc',
        'no_image_plugin' => false,
        'image_plugin' => nil,
        'image_plugin_extra_args' => [],
        'privileged_image_plugin' => nil,
        'privileged_image_plugin_extra_args' => [],
        'network_plugin' => nil,
        'network_plugin_extra_args' => [],
        'additional_bpm_volumes' => [],
        'max_containers' => 250,
        'cpu_quota_per_share_in_us' => 0,
        'experimental_cpu_entitlement_per_share_in_percent' => 0,
        'experimental_tcp_mem_limit_in_bytes' => 0,
        'disable_swap_limit' => false,
        'destroy_containers_on_start' => false,
        'network_pool' => "10.254.0.0/22",
        'http_proxy' => nil,
        'https_proxy' => nil,
        'no_proxy' => nil,
        'apparmor_profile' => 'garden-default',
        'experimental_rootless_mode' => false,
        'cleanup_process_dirs_on_wait' => false,
        'containerd_mode' => false,
        'tcp_keepalive_time' => nil,
        'tcp_keepalive_intvl' => nil,
        'tcp_keepalive_probes' => nil,
        'tcp_retries1' => nil,
        'tcp_retries2' => nil,
        'experimental_cpu_throttling' => false,
        'experimental_use_containerd_mode_for_processes' => false,
        'experimental_cpu_throttling_check_interval' => 15
      },
      'grootfs' => {
        'log_level' => 'info',
        'dropsonde_port' => 3457,
        'insecure_docker_registry_list' => [],
        'skip_mount' => false,
        'graph_cleanup_threshold_in_mb' => -1,
        'tls' => {
          'cert' => nil,
          'key' => nil,
          'ca_cert' => nil
        }
      },
      'reserved_space_for_other_jobs_in_mb' => 15360,
      'experimental_direct_io' => false,
      'bpm.enabled' => false,
      'logging.format.timestamp' => "unix-epoch"
    }
  }

  context 'config/config.ini' do
    context 'with defaults' do
      it 'sets the bind socket' do
        rendered_template = IniParse.parse(template.render(properties))
        expect(rendered_template['server']['bind-socket']).to eql('/var/vcap/data/garden/garden.sock')
      end
    end

    context 'with a listen address' do
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