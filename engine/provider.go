package engine

import "strings"

// ---------------------------------------------------------------------------
// Models in the cloud
//
// Clips are found either by a model on this machine or by a model somebody
// runs for us, paid by the episode. More than one company does the second,
// and nobody should have to have an account with a particular one to use
// the app. So the cloud is a list of providers, each with its own address,
// its own key and its own way of being asked, and the model that is asked
// for says which provider it belongs to. The rest of a search, the prompt,
// the answer read as it is written, the clips taken from it, the repair of
// a broken one, is the same whoever answers.
// ---------------------------------------------------------------------------

// Provider is a company whose API can find clips.
type Provider struct {
	// Name is how the settings and the code know it.
	Name string `json:"name"`
	// Title is how a person knows it.
	Title string `json:"title"`
	// URL is the one address a request for it goes to. It is written here
	// and never derived from any input, so a key is only ever sent to the
	// company it belongs to.
	URL string `json:"-"`
	// Models is the provider's list of models, which is asked for to find
	// out whether a key is taken before it is kept. Written here like URL.
	Models string `json:"-"`
	// Env is the variable the key is read from before the keychain, which
	// is how the command line is given one.
	Env string `json:"env"`
	// Item is the keychain item the key is kept in, under this name, which
	// is what Keychain Access shows. Only the app may read it.
	Item string `json:"-"`
	// Keychain is the item the first keys were kept in, made with the
	// security command, which anything on the machine could read. It is
	// only ever read, to move a key out of it, and removed.
	Keychain string `json:"-"`
	// KeysAt is where a person gets a key.
	KeysAt string `json:"keysAt"`
}

// providers are the companies the app can ask. Tests point the addresses at
// a server of their own, which is the only reason this is a variable.
var providers = []Provider{
	{Name: "anthropic", Title: "Anthropic", URL: "https://api.anthropic.com/v1/messages",
		Models: "https://api.anthropic.com/v1/models",
		Env:    "ANTHROPIC_API_KEY", Item: "Frame Fairy: Anthropic API key", Keychain: "anthropic-api-key",
		KeysAt: "console.anthropic.com"},
	{Name: "openai", Title: "OpenAI", URL: "https://api.openai.com/v1/chat/completions",
		Models: "https://api.openai.com/v1/models",
		Env:    "OPENAI_API_KEY", Item: "Frame Fairy: OpenAI API key", Keychain: "openai-api-key",
		KeysAt: "platform.openai.com"},
}

// Providers lists the companies the app can ask.
func Providers() []Provider {
	return append([]Provider(nil), providers...)
}

// ProviderNamed finds a provider by its name.
func ProviderNamed(name string) (Provider, bool) {
	for _, p := range providers {
		if p.Name == name {
			return p, true
		}
	}
	return Provider{}, false
}

// ProviderFor says which company a model belongs to, by its name. Each
// names its models its own way, and a model this has not heard of is taken
// to be Anthropic's, which is what every model was before there was a
// choice, so a setting from then still means what it meant.
func ProviderFor(model string) Provider {
	name := "anthropic"
	for _, prefix := range []string{"gpt-", "chatgpt-", "o1", "o3", "o4"} {
		if strings.HasPrefix(model, prefix) {
			name = "openai"
		}
	}
	p, _ := ProviderNamed(name)
	return p
}

// CloudModel is a model in the cloud the app offers by name. Any other
// model a provider has can still be asked for, by writing its name in the
// settings or with --model, and ProviderFor sends it to the right place.
type CloudModel struct {
	Model    string `json:"model"`
	Title    string `json:"title"`
	Provider string `json:"provider"`
}

// cloudModels are one model from each provider, the one that finds clips
// best for what it costs. Two of the same price, so the choice between
// them is a choice of company rather than of budget: Claude Sonnet 5 and
// GPT-6 Sol, both 2 dollars a million tokens in and 10 out.
var cloudModels = []CloudModel{
	{Model: "claude-sonnet-5", Title: "Claude Sonnet 5", Provider: "anthropic"},
	{Model: "gpt-6-sol", Title: "GPT-6 Sol", Provider: "openai"},
}

// CloudModels lists the models in the cloud the app offers by name.
func CloudModels() []CloudModel {
	return append([]CloudModel(nil), cloudModels...)
}
