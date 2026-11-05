package launcher

// These are the existing Python JSON defaults, not a new profile schema.
const dataContractMKXPZDefaults = `{
 "rgssVersion":0,"displayFPS":false,"printFPS":false,"winResizable":true,
 "fullscreen":false,"fixedAspectRatio":true,"smoothScaling":0,"smoothScalingDown":0,
 "bitmapSmoothScaling":0,"bitmapSmoothScalingDown":0,"smoothScalingMipmaps":false,
 "bicubicSharpness":100,"xbrzScalingFactor":4.0,"enableHires":false,
 "textureScalingFactor":4.0,"framebufferScalingFactor":4.0,"atlasScalingFactor":4.0,
 "vsync":false,"defScreenW":640,"defScreenH":480,"windowTitle":"Custom Title",
 "fixedFramerate":0,"frameSkip":false,"syncToRefreshrate":false,
 "solidFonts":["Arial","Times New Roman"],"preferMetalRenderer":true,"subImageFix":false,
 "enableBlitting":false,"maxTextureSize":0,"integerScalingActive":false,
 "integerScalingLastMile":true,"gameFolder":"","anyAltToggleFS":false,
 "enableReset":true,"enableSettings":true,"allowSymlinks":true,"dataPathOrg":"mycompany",
 "dataPathApp":"mygame","iconPath":"/path/to/icon.png","customScript":"/path/to/script.rb",
 "preloadScript":[],"postloadScript":["/path/to/script.rb"],"pathCache":true,"RTP":[],
 "patches":["/path/to/patch1.zip"],"useScriptNames":true,
 "fontSub":["Arial>Open Sans","Times New Roman>Liberation Serif"],
 "rubyLoadpath":["/usr/lib64/ruby/"],"JITEnable":false,"JITVerboseLevel":0,
 "JITMaxCache":100,"JITMinCalls":10000,"YJITEnable":false,"midiSoundFont":"",
 "midiChorus":false,"midiReverb":false,"SESourceCount":6,"BGMTrackCount":1,
 "execName":"Game","bindingNames":{"c":"Confirm","b":"Cancel"},"dumpAtlas":false
}`
const dataContractEasyDefaults = `{
 "--autobattle-algo":"RPG_RT","--enemyai-algo":"RPG_RT","--config-path":"",
 "--encoding":"","--engine":"","--font1":"","--font1-size":12,"--font2":"",
 "--font2-size":12,"--font-path":"","--language":"","--load-game-id":"",
 "--log-file":"","--new-game":false,"--no-log-color":false,"--no-rtp":false,
 "--patch-antilag-switch":"","--patch-common-this":false,"--patch-direct-menu":"",
 "--patch-dynrpg":false,"--patch-easyrpg":false,"--patch-key-patch":false,
 "--patch-maniac":1,"--patch-pic-unlock":false,"--patch-rpg2k3-cmds":false,
 "--no-patch":false,"--project-path":"","--record-input":"","--replay-input":"",
 "--rtp-path":"","--save-path":"","--seed":"","--fps-limit":60,
 "--no-fps-limit":false,"--game-resolution":"","--pause-focus-lost":false,
 "--no-pause-focus-lost":false,"--scaling":"","--show-fps":false,
 "--fps-render-window":false,"--no-show-fps":false,"--stretch":false,
 "--no-stretch":false,"--vsync":true,"--no-vsync":false,"--no-audio":false,
 "--music-volume":100,"--sound-volume":100,"--soundfont":"","--soundfont-path":"",
 "--battle-test":"","--hide-title":false,"--start-map-id":"","--start-party":"",
 "--start-position":"","--test-play":false
}`

func DefaultAdvancedSettings() AdvancedSettings {
	mk, err := dataContractObject([]byte(dataContractMKXPZDefaults))
	if err != nil {
		panic(err)
	}
	easy, err := dataContractObject([]byte(dataContractEasyDefaults))
	if err != nil {
		panic(err)
	}
	a := AdvancedSettings{MKXPZ: mk, EasyRPG: easy, MKXPZEnabled: map[string]any{}, EasyRPGEnabled: map[string]any{}}
	for k := range mk {
		a.MKXPZEnabled[k] = false
	}
	for _, k := range []string{"rgssVersion", "gameFolder", "preloadScript", "RTP", "midiSoundFont"} {
		a.MKXPZEnabled[k] = "Force"
	}
	for k := range easy {
		if k != "--project-path" {
			a.EasyRPGEnabled[k] = false
		}
	}
	return a
}
