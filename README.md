# RPG-Maker-MacOS-Launcher
A launcher to natively play RPG maker MV, MZ, XP, VX, and VX Ace games on MacOS!<br>
Development for this launcher has been completed! Only bugfixes will be released if needed.<br>
<br>
Plan/TODO (no particular order)
1. ✅ Add nwjs version selected<br>
2. ✅ Toggle for game_en.exe extractor (if for some reason you want to play in japanese?!)<br>
3. ✅ Toggle for cheat menu<br>
4. ✅ Better logging incase something goes wrong<br>
5. ✅ Remember last opened game<br>
6. ❌ Look in directory(s) for a bunch of games<br>
7. ✅ Export game as standalone app<br>
8. ❌ Better looking UI<br>
9. ✅ Better looking Icon<br>
10. ✅ Save editor<br>
11. ✅ mkxp-z (support for Rpg Maker XP / VX / VX Ace)<br>
12. ✅ Export non MV or MZ game as standalone app<br>
13. ✅ Add Midi support<br>
14. ✅ Add advanced options for mkxp-z<br>
15. ✅ Apply advanced options and midi when exporting mkxp-z games

# How it works
1. Downloads nwjs and mkxp-z
2. Checks package.json or Game.ini
3. Unpacks game_en.exe if it exists
4. Points nwjs or mkxp-z to directory
5. Launches game
# How to build
Native version (Specifically x64 or arm64 depending on your system):
1. Clone the repo <br> `git clone https://github.com/m5kro/RPG-Maker-MacOS-Launcher`
2. Setup the venv <br> `python3 -m venv RPGM-Launcher` <br> `source RPGM-Launcher/bin/activate`
3. Install packages <br> `pip install PySide6-Essentials evbunpack requests chardet PyInstaller`
4. Build the app <br> `pyinstaller --name "RPG-Maker-Launcher" --add-data Cheat_Menu.js:. --add-data Cheat_Menu.css:. --icon ./icon.icns --onedir --windowed main.py`
5. Find the app in the dist folder
6. Decativate venv <br> `deactivate`
7. Clear the venv <br> `sudo rm -rf venv`

Universal2 version (works on both x64 or arm64):
1. Download and install Universal2 version of python: https://www.python.org/downloads/macos/
2. Clone the repo <br> `git clone https://github.com/m5kro/RPG-Maker-MacOS-Launcher`
3. Setup the venv <br> `python3 -m venv RPGM-Launcher` <br> `source RPGM-Launcher/bin/activate`
4. Install packages <br> `pip install PySide6-Essentials evbunpack requests PyInstaller`
5. Replace charset-normalizer with chardet <br> `pip uninstall charset-normalizer` <br> `pip install chardet`
6. Build the app <br> `pyinstaller --name "RPG-Maker-Launcher" --add-data Cheat_Menu.js:. --add-data Cheat_Menu.css:. --icon ./icon.icns --target-arch universal2 --onedir --windowed main.py`
7. Find the app in the dist folder
8. Decativate venv <br> `deactivate`
9. Clear the venv <br> `sudo rm -rf venv`
# Credits and Special Thanks
Donators:<br>
lecrolonk <br>
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
# Donations
Bitcoin Wallet:<br>
bc1qup498tqtm55zav2ckjyhlq8qnd0xmwjnvs7fa3<br>
<br>
Bitcoin Lightning Wallet (not an email address!):<br>
m5kro@speed.app<br>
<br>
Ethereum Wallet:<br>
0x60F040444876EB5996DfA04CB8d8fc8D2aB96CF7
