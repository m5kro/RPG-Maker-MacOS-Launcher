# RPG-Maker-MV-MZ-MacOS-Launcher
A launcher to natively play RPG maker MV and MZ games on MacOS!<br>
<br>
# How it works
1. Downloads nwjs
2. Checks package.json
3. Unpacks game_en.exe if it exists
4. Points nwjs to directory
5. Launches game
# How to build
Native version (Specifically x64 or arm64 depending on your system):
1. Clone the repo <br> `git clone https://github.com/m5kro/RPG-Maker-MV-MZ-MacOS-Launcher/`
2. Setup the venv <br> `python3 -m venv RPGM-Launcher` <br> `source RPGM-Launcher/bin/activate`
3. Install packages <br> `pip install PySide6-Essentials evbunpack requests chardet PyInstaller`
4. Build the app <br> `pyinstaller --name "RPG-Maker-Launcher" --add-data Cheat_Menu.js:. --add-data Cheat_Menu.css:. --icon ./icon.icns --onedir --windowed main.py`
5. Find the app in the dist folder
6. Decativate venv <br> `deactivate`
7. Clear the venv <br> `sudo rm -rf venv`

Universal2 version (works on both x64 or arm64):
1. Download and install Universal2 version of python: https://www.python.org/downloads/macos/
2. Clone the repo <br> `git clone https://github.com/m5kro/RPG-Maker-MV-MZ-MacOS-Launcher/`
3. Setup the venv <br> `python3 -m venv RPGM-Launcher` <br> `source RPGM-Launcher/bin/activate`
4. Install packages <br> `pip install PySide6-Essentials evbunpack requests PyInstaller`
5. Replace charset-normalizer with chardet <br> `pip uninstall charset-normalizer` <br> `pip install chardet`
6. Build the app <br> `pyinstaller --name "RPG-Maker-Launcher" --add-data Cheat_Menu.js:. --add-data Cheat_Menu.css:. --icon ./icon.icns --target-arch universal2 --onedir --windowed main.py`
7. Find the app in the dist folder
8. Decativate venv <br> `deactivate`
9. Clear the venv <br> `sudo rm -rf venv`
# Credits
Credit to emerladCoder for cheat menu <br>
https://github.com/emerladCoder/RPG-Maker-MV-Cheat-Menu-Plugin