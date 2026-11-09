<img src="icon.png" height="200">

# RPG-Maker-MacOS-Launcher
> [!NOTE]
> Development for this launcher has been completed! Only bugfixes will be released if needed.<br>
> Check out [Xenolauncher](https://github.com/m5kro/Xenolauncher) for a more versatile launcher!

A launcher to natively play RPG maker MV, MZ, XP, VX, VX Ace, 2000, and 2003 games on MacOS!<br>
Custom MKXP-Z fork: https://github.com/m5kro/mkxp-z/tree/RPGM-Launcher<br>

> [!TIP]
> Instructions video by [xem](https://forums.rpgmakerweb.com/index.php?members/xem.159435/)/[pgm](https://www.youtube.com/channel/UCWTsqPj8SD8rHL9gsBZI59Q): https://www.youtube.com/watch?v=NIRiKIjhXHk
<br>

# Donations
Ethereum Wallet:<br>
0x60F040444876EB5996DfA04CB8d8fc8D2aB96CF7

# How it works
1. Downloads nwjs, mkxp-z, and EasyRPG
2. Checks package.json, Game.ini, or RTP_RT
3. Unpacks game_en.exe if it exists
4. Points nwjs, mkxp-z, or EasyRPG to directory
5. Launches game

## Shared NW.js exports
Exports are standalone by default. To save disk space, enable **NW.js export: use shared runtime** before exporting an MV/MZ game. The app contains its own copy of the game, but uses the exact NW.js installation and Rosetta choice selected at export time. The option is remembered; MKXP-Z and EasyRPG exports are always standalone.

Shared exports use runtimes installed in the exporting account. If the selected version is missing, choose another installed version or install one in RPG Maker Launcher. No runtime is downloaded automatically. After selecting a replacement, leave **Skip future confirmation** unchecked to choose or confirm a version on each launch; checking it launches that version without asking while it remains installed. Missing runtimes always prompt again. To reset a skipped choice, remove the exported app's preferences using `defaults delete <bundle identifier>` (the identifier is in its `Contents/Info.plist`), or remove the selected runtime. A different NW.js version may not be compatible with the game. Source-game extraction, cheat and optimization options still operate on the original game before copying. Game files, including saves stored inside the game, are separate from the source. As with other exports, macOS security permissions may affect launching.

# How to build
If the [prebuilt releases](https://github.com/m5kro/RPG-Maker-MacOS-Launcher/releases/latest) don't work, or you'd like to add a feature, here are instructions on how to build the app:<br><br>
Recommended Python version is 3.11<br>
<br>
Native version (Specifically x64 or arm64 depending on your system):
1. Clone the repo <br> `git clone https://github.com/m5kro/RPG-Maker-MacOS-Launcher` <br> `cd RPG-Maker-MacOS-Launcher`
2. Setup the venv <br> `python3 -m venv RPGM-Launcher` <br> `source RPGM-Launcher/bin/activate`
3. Install packages <br> `pip install PySide6-Essentials evbunpack requests chardet PyInstaller`
4. Build the app <br> `pyinstaller RPG-Maker-Launcher-(arm64 or x64).spec`
5. Find the app in the dist folder
6. Decativate venv <br> `deactivate`
7. Clear the venv <br> `sudo rm -rf venv`

Universal2 version (works on both x64 or arm64):
1. Download and install Universal2 version of python: https://www.python.org/downloads/macos/
2. Clone the repo <br> `git clone https://github.com/m5kro/RPG-Maker-MacOS-Launcher` <br> `cd RPG-Maker-MacOS-Launcher`
3. Setup the venv <br> `python3 -m venv RPGM-Launcher` <br> `source RPGM-Launcher/bin/activate`
4. Install packages <br> `pip install PySide6-Essentials evbunpack requests PyInstaller`
5. Replace charset-normalizer with chardet <br> `pip uninstall charset-normalizer` <br> `pip install chardet`
6. Build the app <br> `pyinstaller RPG-Maker-Launcher-Universal2.spec`
7. Find the app in the dist folder
8. Decativate venv <br> `deactivate`
9. Clear the venv <br> `sudo rm -rf venv`
# Credits and Special Thanks
Donators:<br>
lecrolonk <br>
athanadoc05 <br>
<br>
Credit to mkxp-z maintainers for mkxp-z <br>
https://github.com/mkxp-z/mkxp-z <br>
<br>
Credit to emerladCoder for cheat menu <br>
https://github.com/emerladCoder/RPG-Maker-MV-Cheat-Menu-Plugin <br>
<br>
Credit to SaveEditorOnline for the save editor <br>
https://saveeditor.online <br>
<br>
Credit to SynthFont developers for the Soundfont <br>
https://musical-artifacts.com/artifacts/841 <br>
<br>
Credit to Orochimarufan for the Kawariki Patches <br>
https://github.com/Orochimarufan/Kawariki <br>
<br>
Credit to EasyRPG team for EasyRPG Player<br>
https://easyrpg.org/player/downloads/#release-macos<br>
<br>

