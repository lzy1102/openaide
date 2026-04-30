package config

import "testing"

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "empty home_dir",
			config: &Config{
				Models: []ModelConfig{{Name: "test", APIKey: "key"}},
			},
			wantErr: true,
		},
		{
			name: "no models",
			config: &Config{
				HomeDir: "/opt/openaide",
				Models:  []ModelConfig{},
			},
			wantErr: true,
		},
		{
			name: "model without name",
			config: &Config{
				HomeDir: "/opt/openaide",
				Models:  []ModelConfig{{APIKey: "key"}},
			},
			wantErr: true,
		},
		{
			name: "model without api_key",
			config: &Config{
				HomeDir: "/opt/openaide",
				Models:  []ModelConfig{{Name: "test"}},
			},
			wantErr: true,
		},
		{
			name: "valid config",
			config: &Config{
				HomeDir: "/opt/openaide",
				Storage: StorageConfig{
					DB:          DBConfig{Type: "sqlite", URI: "/opt/openaide/data/db/openaide.db"},
					Cache:       CacheConfig{Type: "memory"},
					VectorStore: VectorStoreConfig{Type: "hnsw"},
				},
				Models: []ModelConfig{{Name: "test", APIKey: "key"}},
			},
			wantErr: false,
		},
		{
			name: "invalid db type",
			config: &Config{
				HomeDir: "/opt/openaide",
				Storage: StorageConfig{
					DB:          DBConfig{Type: "mongodb"},
					Cache:       CacheConfig{Type: "memory"},
					VectorStore: VectorStoreConfig{Type: "hnsw"},
				},
				Models: []ModelConfig{{Name: "test", APIKey: "key"}},
			},
			wantErr: true,
		},
		{
			name: "invalid cache type",
			config: &Config{
				HomeDir: "/opt/openaide",
				Storage: StorageConfig{
					DB:          DBConfig{Type: "sqlite", URI: "/opt/openaide/data/db/openaide.db"},
					Cache:       CacheConfig{Type: "memcached"},
					VectorStore: VectorStoreConfig{Type: "hnsw"},
				},
				Models: []ModelConfig{{Name: "test", APIKey: "key"}},
			},
			wantErr: true,
		},
		{
			name: "invalid vector store type",
			config: &Config{
				HomeDir: "/opt/openaide",
				Storage: StorageConfig{
					DB:          DBConfig{Type: "sqlite", URI: "/opt/openaide/data/db/openaide.db"},
					Cache:       CacheConfig{Type: "memory"},
					VectorStore: VectorStoreConfig{Type: "unknown"},
				},
				Models: []ModelConfig{{Name: "test", APIKey: "key"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStorageConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  StorageConfig
		wantErr bool
	}{
		{
			name: "valid sqlite",
			config: StorageConfig{
				DB:          DBConfig{Type: "sqlite", URI: "/opt/openaide/data/db/openaide.db"},
				Cache:       CacheConfig{Type: "memory"},
				VectorStore: VectorStoreConfig{Type: "hnsw"},
			},
			wantErr: false,
		},
		{
			name: "sqlite without uri",
			config: StorageConfig{
				DB:          DBConfig{Type: "sqlite"},
				Cache:       CacheConfig{Type: "memory"},
				VectorStore: VectorStoreConfig{Type: "hnsw"},
			},
			wantErr: true,
		},
		{
			name: "valid postgres",
			config: StorageConfig{
				DB:          DBConfig{Type: "postgres", URI: "postgres://user:pass@localhost/db"},
				Cache:       CacheConfig{Type: "ledis"},
				VectorStore: VectorStoreConfig{Type: "memory"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
