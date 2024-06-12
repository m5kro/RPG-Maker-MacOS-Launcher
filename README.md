# RPG-Maker-MV-MZ-MacOS-Launcher
A launcher to natively play RPG maker MV and MZ games on MacOS! (currently intel x64 only)<br>
<br>
# How it works
1. Downloads nwjs
2. Checks package.json
3. Unpacks game_en.exe if it exists
4. Points nwjs to directory
5. Launches game
# How to compile
1. Clone the repo <br> `git clone https://github.com/m5kro/RPG-Maker-MV-MZ-MacOS-Launcher/`
2. Setup the venv <br> `python3 -m venv RPGM-Launcher` <br> `source RPGM-Launcher/bin/activate`
3. Install packages <br> `pip install PySide6-Essentials evbunpack requests PyInstaller`
4. Build the app <br> `pyinstaller --name "RPG-Maker-Launcher" --icon ./icon.icns --onefile --windowed main.py`
5. Find the app in the dist folder
