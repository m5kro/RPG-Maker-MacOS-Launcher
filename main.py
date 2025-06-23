import sys
import os
import json
import subprocess
import requests
import zipfile
import shutil
import platform
import chardet
import logging
import re
from functools import partial
from evbunpack.__main__ import unpack_files
from PySide6.QtWidgets import (QApplication, QMainWindow, QPushButton, QFileDialog,
                               QVBoxLayout, QHBoxLayout, QWidget, QMessageBox, QDialog, QComboBox,
                               QDialogButtonBox, QScrollArea, QGroupBox, QFormLayout, QLabel, QCheckBox, QProgressDialog, QLineEdit, QPlainTextEdit)
from PySide6.QtCore import QTimer, QDateTime, Qt

current_version = "3.0"
config_version = ""
latest_commit_sha = ""
last_commit_sha = ""

# Default settings for MKXP-Z
default_enabled_settings = {
            "rgssVersion": "Force", "displayFPS": False, "printFPS": False, "winResizable": False,
            "fullscreen": False, "fixedAspectRatio": False, "smoothScaling": False,
            "smoothScalingDown": False, "bitmapSmoothScaling": False, "bitmapSmoothScalingDown": False,
            "smoothScalingMipmaps": False, "bicubicSharpness": False, "xbrzScalingFactor": False,
            "enableHires": False, "textureScalingFactor": False, "framebufferScalingFactor": False,
            "atlasScalingFactor": False, "vsync": False, "defScreenW": False, "defScreenH": False,
            "windowTitle": False, "fixedFramerate": False, "frameSkip": False, "syncToRefreshrate": False,
            "solidFonts": False, "preferMetalRenderer": False, "subImageFix": False, "enableBlitting": False,
            "maxTextureSize": False, "integerScalingActive": False, "integerScalingLastMile": False,
            "gameFolder": "Force", "anyAltToggleFS": False, "enableReset": False, "enableSettings": False,
            "allowSymlinks": False, "dataPathOrg": False, "dataPathApp": False, "iconPath": False,
            "customScript": False, "preloadScript": "Force", "postloadScript": False, "pathCache": False,
            "RTP": "Force", "patches": False, "useScriptNames": False, "fontSub": False, "rubyLoadpath": False,
            "JITEnable": False, "JITVerboseLevel": False, "JITMaxCache": False, "JITMinCalls": False,
            "YJITEnable": False, "midiSoundFont": "Force", "midiChorus": False, "midiReverb": False,
            "SESourceCount": False, "BGMTrackCount": False, "execName": False, "bindingNames": False,
            "dumpAtlas": False
        }

default_advanced_settings = {
            "rgssVersion": 0, "displayFPS": False, "printFPS": False, "winResizable": True,
            "fullscreen": False, "fixedAspectRatio": True, "smoothScaling": 0,
            "smoothScalingDown": 0, "bitmapSmoothScaling": 0, "bitmapSmoothScalingDown": 0,
            "smoothScalingMipmaps": False, "bicubicSharpness": 100, "xbrzScalingFactor": 4.0,
            "enableHires": False, "textureScalingFactor": 4.0, "framebufferScalingFactor": 4.0,
            "atlasScalingFactor": 4.0, "vsync": False, "defScreenW": 640, "defScreenH": 480,
            "windowTitle": "Custom Title", "fixedFramerate": 0, "frameSkip": False,
            "syncToRefreshrate": False, "solidFonts": ["Arial", "Times New Roman"],
            "preferMetalRenderer": True, "subImageFix": False, "enableBlitting": False,
            "maxTextureSize": 0, "integerScalingActive": False, "integerScalingLastMile": True,
            "gameFolder": "", "anyAltToggleFS": False, "enableReset": True, "enableSettings": True,
            "allowSymlinks": True, "dataPathOrg": "mycompany", "dataPathApp": "mygame",
            "iconPath": "/path/to/icon.png", "customScript": "/path/to/script.rb",
            "preloadScript": [],
            "postloadScript": ["/path/to/script.rb"], "pathCache": True, "RTP": [],
            "patches": ["/path/to/patch1.zip"], "useScriptNames": True, "fontSub": [
                "Arial>Open Sans", "Times New Roman>Liberation Serif"],
            "rubyLoadpath": ["/usr/lib64/ruby/"], "JITEnable": False, "JITVerboseLevel": 0,
            "JITMaxCache": 100, "JITMinCalls": 10000, "YJITEnable": False, "midiSoundFont": "",
            "midiChorus": False, "midiReverb": False, "SESourceCount": 6, "BGMTrackCount": 1,
            "execName": "Game", "bindingNames": {"c": "Confirm", "b": "Cancel"},
            "dumpAtlas": False
        }

def check_appdir():
    app_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher")
    if not os.path.exists(app_dir):
        os.makedirs(app_dir)
    # Clear log file on startup
    with open(LOG_FILE, 'w') as file:
        file.write("")

# Set up logging
LOG_FILE = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/log.txt")

class FolderPathApp(QMainWindow):
    SETTINGS_FILE = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/settings.json")

    def __init__(self):
        super().__init__()

        self.setWindowTitle("RPG Maker MV and MZ Launcher")
        self.resize(400, 200)

        self.central_widget = QWidget()
        self.setCentralWidget(self.central_widget)

        self.layout = QVBoxLayout(self.central_widget)

        self.instructions_button = QPushButton("Instructions", self)
        self.layout.addWidget(self.instructions_button)
        self.instructions_button.clicked.connect(self.show_instructions)

        self.credits_button = QPushButton("Credits", self)
        self.layout.addWidget(self.credits_button)
        self.credits_button.clicked.connect(self.show_credits)

        version_layout = QHBoxLayout()
        self.version_label = QLabel("NWJS version:", self)
        version_layout.addWidget(self.version_label)

        self.version_selector = QComboBox(self)
        version_layout.addWidget(self.version_selector)
        self.layout.addLayout(version_layout)

        self.extract_checkbox = QCheckBox("Extract game_en.exe", self)
        self.layout.addWidget(self.extract_checkbox)

        self.cheat_menu_checkbox = QCheckBox("Add Cheat Menu (Press [1] key to open)", self)
        self.layout.addWidget(self.cheat_menu_checkbox)

        self.optimize_space_checkbox = QCheckBox("Optimize Space", self)
        self.layout.addWidget(self.optimize_space_checkbox)

        self.selected_folder_label = QLabel("No folder selected", self)
        self.layout.addWidget(self.selected_folder_label)

        self.select_button = QPushButton("Select Game Folder", self)
        self.layout.addWidget(self.select_button)

        self.start_game_button = QPushButton("Start Game", self)
        self.layout.addWidget(self.start_game_button)
        self.start_game_button.setEnabled(False)  # Disable until conditions are met

        self.export_button = QPushButton("Export as Standalone App", self)
        self.layout.addWidget(self.export_button)
        self.export_button.setEnabled(False)  # Disable until conditions are met

        self.open_save_editor_button = QPushButton("Open Save Editor", self)
        self.layout.addWidget(self.open_save_editor_button)

        self.install_button = QPushButton("Install NWJS Version", self)
        self.layout.addWidget(self.install_button)

        self.uninstall_button = QPushButton("Uninstall NWJS Version", self)
        self.layout.addWidget(self.uninstall_button)

        self.mkxpz_advanced_button = QPushButton("MKXP-Z Advanced Settings", self)
        self.layout.addWidget(self.mkxpz_advanced_button)

        self.open_log_button = QPushButton("Open Log", self)
        self.layout.addWidget(self.open_log_button)

        self.install_button.clicked.connect(self.install_nwjs)
        self.uninstall_button.clicked.connect(self.uninstall_nwjs)
        self.select_button.clicked.connect(self.select_folder)
        self.start_game_button.clicked.connect(self.start_game)
        self.export_button.clicked.connect(self.export_standalone_app)
        self.open_save_editor_button.clicked.connect(self.open_save_editor)
        self.mkxpz_advanced_button.clicked.connect(self.open_mkxpz_advanced_settings)
        self.open_log_button.clicked.connect(self.open_log)

        self.update_version_selector()
        self.load_settings()
        if current_version != config_version:
            self.remove_configs()
        self.update_select_button_state()
        self.check_start_game_button()

    def show_instructions(self):
        instructions = (
            "RPG Maker MV and MZ games:\n\n"
            "1. Click 'Install NWJS Version' to download and install the required version of NWJS.\n\n"
            "2. Click 'Select Game Folder' to choose the folder containing the RPG Maker game.\n\n"
            "3. Check Options:\n"
            "   - 'Extract game_en.exe': Extracts the file and patches the game with the English version.\n"
            "   - 'Add Cheat Menu': Patch the game with a cheat menu (Press [1] key while in game).\n"
            "   - 'Optimize Space': Optimize the game folder size by removing the Windows version of NWJS.\n\n"
            "4. Click 'Start Game' to launch the game using the selected NWJS version.\n\n"
            "5. Click 'Export as Standalone App' to create a standalone application for the game.\n\n"
            "6. Click 'Open Save Editor' to open the save editor website and the save folder.\n\n"
            "7. Click 'Uninstall NWJS Version' to remove an installed version of NWJS.\n"
            "_____________________________________________________________________________\n"
            "RPG Maker 2000, 2003, XP, VX, and VX Ace games:\n\n"
            "1. Move game folder to anywhere but the 'Downloads' folder. (XP, VX, VX Ace only)\n\n"
            "2. Click 'Select Game Folder' to choose the folder containing the RPG Maker game.\n\n"
            "3. Click 'Start Game' to launch the game. You may be prompted to install MKXP-Z or EasyRPG."
        )
        
        dialog = QDialog(self)
        dialog.setWindowTitle("Instructions")
        dialog.resize(650, 525)
        layout = QVBoxLayout(dialog)

        text_edit = QPlainTextEdit(dialog)
        text_edit.setPlainText(instructions)
        text_edit.setReadOnly(True)
        layout.addWidget(text_edit)

        buttons = QDialogButtonBox(QDialogButtonBox.Ok, dialog)
        buttons.accepted.connect(dialog.accept)
        layout.addWidget(buttons)

        dialog.exec()

    # <3
    def show_credits(self):
        credits = (
            "Special Thanks To:\n\n"
            "lecrolonk - Donator\n\n"
            "mkxp-z maintainers - MKXP-Z\n\n"
            "Andmi Kuzgri - Save Editor Online\n\n"
            "emerladCoder - Cheat Menu Plugin\n\n"
            "SynthFont developers - Soundfont\n\n"
            "Orochimarufan - Kawariki Patches\n\n"
            "EasyRPG Team - EasyRPG Player"
        )
        
        dialog = QDialog(self)
        dialog.setWindowTitle("Credits")
        dialog.resize(650, 340)
        layout = QVBoxLayout(dialog)

        text_edit = QPlainTextEdit(dialog)
        text_edit.setPlainText(credits)
        text_edit.setReadOnly(True)
        layout.addWidget(text_edit)

        buttons = QDialogButtonBox(QDialogButtonBox.Ok, dialog)
        buttons.accepted.connect(dialog.accept)
        layout.addWidget(buttons)

        dialog.exec()

    def load_settings(self):
        if os.path.exists(self.SETTINGS_FILE):
            try:
                with open(self.SETTINGS_FILE, 'r') as file:
                    settings = json.load(file)
                    self.extract_checkbox.setChecked(settings.get('extract_game_en', False))
                    self.cheat_menu_checkbox.setChecked(settings.get('add_cheat_menu', False))
                    self.optimize_space_checkbox.setChecked(settings.get('optimize_space', False))
                    last_version = settings.get('last_selected_version')
                    if last_version and last_version in [self.version_selector.itemText(i) for i in range(self.version_selector.count())]:
                        self.version_selector.setCurrentText(last_version)
                    self.last_selected_folder = settings.get('last_selected_folder', os.path.expanduser("~/Downloads"))
                    global config_version
                    config_version = settings.get('launcher_version', "")
                    global last_commit_sha
                    last_commit_sha = settings.get('last_commit_sha', "")
                    self.update_selected_folder_label()
            except Exception as e:
                logging.error("Failed to load settings: %s", str(e))
                logging.error("Using default settings.")
                self.last_selected_folder = os.path.expanduser("~/Downloads")
                self.update_selected_folder_label()
        else:
            self.last_selected_folder = os.path.expanduser("~/Downloads")
            self.update_selected_folder_label()

    def save_settings(self):
        settings = {
            'extract_game_en': self.extract_checkbox.isChecked(),
            'add_cheat_menu': self.cheat_menu_checkbox.isChecked(),
            'optimize_space': self.optimize_space_checkbox.isChecked(),
            'last_selected_version': self.version_selector.currentText(),
            'last_selected_folder': self.last_selected_folder,
            'launcher_version': current_version,
            'last_commit_sha': last_commit_sha
        }
        with open(self.SETTINGS_FILE, 'w') as file:
            json.dump(settings, file, indent=4)
    
    # Remove old MKXP-Z configuration files every update
    def remove_configs(self):
        enabled_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/enabled-mkxpz-settings.json")
        advanced_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/mkxpz-advanced.json")
        if os.path.exists(enabled_settings_file):
            os.remove(enabled_settings_file)
        if os.path.exists(advanced_settings_file):
            os.remove(advanced_settings_file)
        logging.info("Removed old MKXP-Z configuration files.")

    def open_mkxpz_advanced_settings(self):
        dialog = QDialog(self)
        dialog.setWindowTitle("MKXP-Z Advanced Settings")
        dialog.resize(500, 700)

        scroll_area = QScrollArea(dialog)
        scroll_area.setWidgetResizable(True)
        scroll_content = QWidget()
        scroll_layout = QVBoxLayout(scroll_content)
        scroll_area.setWidget(scroll_content)

        enabled_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/enabled-mkxpz-settings.json")
        advanced_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/mkxpz-advanced.json")

        if not os.path.exists(enabled_settings_file):
            with open(enabled_settings_file, 'w') as file:
                json.dump(default_enabled_settings, file, indent=4)
            logging.info("Created enabled-mkxpz-settings.json with default values.")
        
        if not os.path.exists(advanced_settings_file):
            with open(advanced_settings_file, 'w') as file:
                json.dump(default_advanced_settings, file, indent=4)
            logging.info("Created mkxpz-advanced.json with default values.")

        with open(enabled_settings_file, 'r') as file:
            enabled_settings = json.load(file)
        
        with open(advanced_settings_file, 'r') as file:
            advanced_settings = json.load(file)

        required_box = QGroupBox("Required Options, leave blank for default (except rgssVersion which should be 0)", dialog)
        required_layout = QFormLayout(required_box)

        optional_box = QGroupBox("Optional Options", dialog)
        optional_layout = QVBoxLayout(optional_box)

        self.settings_widgets = {}
        for key, enabled in enabled_settings.items():
            # Place "Force" settings in the required box
            if enabled == "Force":
                widget = self.create_setting_widget(advanced_settings[key], required_box)
                required_layout.addRow(f"{key}:", widget)
                self.settings_widgets[key] = {"widget": widget, "enabled_checkbox": None}
            else:
                group_box = QGroupBox(key, dialog)
                group_layout = QFormLayout(group_box)

                enabled_checkbox = QCheckBox("Enabled", group_box)
                enabled_checkbox.setChecked(enabled == "True")
                group_layout.addRow(enabled_checkbox)
                enabled_checkbox.stateChanged.connect(partial(self.update_enabled_state, key, enabled_checkbox))

                widget = self.create_setting_widget(advanced_settings[key], group_box)
                group_layout.addRow("Value:", widget)

                self.settings_widgets[key] = {"widget": widget, "enabled_checkbox": enabled_checkbox}
                optional_layout.addWidget(group_box)

        scroll_layout.addWidget(required_box)
        scroll_layout.addWidget(optional_box)

        buttons_layout = QHBoxLayout()
        save_button = QPushButton("Save", dialog)
        reset_button = QPushButton("Reset", dialog)
        save_button.clicked.connect(lambda: self.save_mkxpz_advanced_settings(enabled_settings_file, advanced_settings_file))
        reset_button.clicked.connect(self.reset_mkxpz_advanced_settings)

        buttons_layout.addWidget(save_button)
        buttons_layout.addWidget(reset_button)

        dialog_layout = QVBoxLayout(dialog)
        dialog_layout.addWidget(scroll_area)
        dialog_layout.addLayout(buttons_layout)
        dialog.setLayout(dialog_layout)

        dialog.exec()

    def create_setting_widget(self, value, parent):
        widget = None
        if isinstance(value, bool):
            widget = QCheckBox(parent)
            widget.setChecked(value)
        elif isinstance(value, (int, float)):
            widget = QLineEdit(parent)
            widget.setText(str(value))
        elif isinstance(value, str):
            widget = QLineEdit(parent)
            widget.setText(value)
        elif isinstance(value, list):
            widget = QPlainTextEdit(parent)
            widget.setPlainText("\n".join(map(str, value)))
        elif isinstance(value, dict):
            widget = QPlainTextEdit(parent)
            widget.setPlainText(json.dumps(value, indent=4))
        return widget


    def update_enabled_state(self, key, checkbox):
        self.settings_widgets[key]["enabled_checkbox"].setChecked(checkbox.isChecked())

    def save_mkxpz_advanced_settings(self, enabled_settings_file, advanced_settings_file):
        enabled_settings = {}
        advanced_settings = {}

        for key, controls in self.settings_widgets.items():
            widget = controls["widget"]
            enabled_checkbox = controls["enabled_checkbox"]

            if key in default_enabled_settings and default_enabled_settings[key] == "Force":
                enabled_settings[key] = "Force"
            elif enabled_checkbox:
                enabled_settings[key] = "True" if enabled_checkbox.isChecked() else "False"

            # Save the current values of advanced settings based on widget types
            if isinstance(widget, QCheckBox):
                advanced_settings[key] = widget.isChecked()
            elif isinstance(widget, QLineEdit):
                value = widget.text()
                if value.isdigit():
                    advanced_settings[key] = int(value)
                else:
                    try:
                        advanced_settings[key] = float(value)
                    except ValueError:
                        advanced_settings[key] = value
            elif isinstance(widget, QPlainTextEdit):
                text = widget.toPlainText().strip()
                try:
                    advanced_settings[key] = json.loads(text)
                except json.JSONDecodeError:
                    advanced_settings[key] = text.splitlines()

        with open(enabled_settings_file, 'w') as file:
            json.dump(enabled_settings, file, indent=4)
        with open(advanced_settings_file, 'w') as file:
            json.dump(advanced_settings, file, indent=4)
        QMessageBox.information(self, "Settings Saved", "MKXP-Z settings have been saved successfully.")

    def reset_mkxpz_advanced_settings(self):
        for key, controls in self.settings_widgets.items():
            widget = controls["widget"]
            enabled_checkbox = controls["enabled_checkbox"]

            default_enabled = default_enabled_settings.get(key, False)
            default_value = default_advanced_settings.get(key)

            if enabled_checkbox:
                enabled_checkbox.setChecked(default_enabled == "True")

            if isinstance(widget, QCheckBox):
                widget.setChecked(default_value)
            elif isinstance(widget, QLineEdit):
                widget.setText(str(default_value))
            elif isinstance(widget, QPlainTextEdit):
                if isinstance(default_value, list):
                    widget.setPlainText("\n".join(map(str, default_value)))
                elif isinstance(default_value, dict):
                    widget.setPlainText(json.dumps(default_value, indent=4))

    def closeEvent(self, event):
        self.save_settings()
        super().closeEvent(event)

    def check_nwjs_installed(self):
        applications_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher")
        return os.path.exists(applications_dir) and any(os.path.isdir(os.path.join(applications_dir, v)) for v in os.listdir(applications_dir))

    def check_mkxpz_installed(self, warn=True):
        mkxpz_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/Z-universal.app")
        if not os.path.exists(mkxpz_path):
            if not warn:
                return False
            install_response = QMessageBox.question(self, "MKXPZ Not Installed", 
                                                    "MKXPZ is not installed. Would you like to install it now?", 
                                                    QMessageBox.Yes | QMessageBox.No)
            if install_response == QMessageBox.Yes:
                self.install_mkxpz()
                return True
            else:
                logging.info("MKXPZ installation canceled by the user.")
                QMessageBox.information(self, "Installation Canceled", "MKXPZ installation was canceled.")
                return False
        return True

    # RTP stands for "Run Time Package", which is a set of resources required by RPG Maker VX Ace, VX, and XP games
    def check_RTP_installed(self):
        rtp_folder = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/RTP")
        if not os.path.exists(rtp_folder):
            install_response = QMessageBox.question(self, "RTP Not Found", 
                                                    "RTP is required but not found. Would you like to install it now?", 
                                                    QMessageBox.Yes | QMessageBox.No)
            if install_response == QMessageBox.Yes:
                self.install_RTP()
                return True
            else:
                logging.info("RTP installation canceled by the user.")
                QMessageBox.information(self, "Installation Canceled", "RTP installation was canceled.")
                return False
        return True
    
    # Soundfont is a file that contains audio samples used for MIDI playback in RPG Maker games
    def check_soundfont_installed(self):
        soundfont_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/GMGSx.SF2")
        if not os.path.exists(soundfont_path):
            install_response = QMessageBox.question(
                self, "Soundfont Not Found",
                "The required soundfont (GMGSx.SF2) is missing. Would you like to download it now?",
                QMessageBox.Yes | QMessageBox.No
            )
            if install_response == QMessageBox.Yes:
                self.download_soundfont()
                return os.path.exists(soundfont_path)
            else:
                logging.info("Soundfont installation canceled by the user.")
                QMessageBox.information(self, "Installation Canceled", "Soundfont installation was canceled.")
                return False
        return True
    
    # Kawariki is a set of patches that improve compatability with RPG Maker games by supplying missing API functions
    def check_kawariki_installed(self):
        kawariki_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/kawariki/preload.rb")
        if not os.path.exists(kawariki_path):
            install_response = QMessageBox.question(
                self, "Kawariki Patches Not Found", 
                "Kawariki Patches are required but not found. Would you like to install it now?", 
                QMessageBox.Yes | QMessageBox.No
            )
            if install_response == QMessageBox.Yes:
                self.download_kawariki_patches()
                return True
            else:
                logging.info("Kawariki installation canceled by the user.")
                QMessageBox.information(self, "Installation Canceled", "Kawariki installation was canceled.")
                return False
        return True
    
    def check_easyrpg_installed(self):
        easyrpg_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/EasyRPG Player.app")
        if not os.path.exists(easyrpg_path):
            install_response = QMessageBox.question(
                self, "EasyRPG Not Found", 
                "EasyRPG is required but not found. Would you like to install it now?", 
                QMessageBox.Yes | QMessageBox.No
            )
            if install_response == QMessageBox.Yes:
                self.install_easyrpg()
                return True
            else:
                logging.info("EasyRPG installation canceled by the user.")
                QMessageBox.information(self, "Installation Canceled", "EasyRPG installation was canceled.")
                return False
        return True
    
    def check_mkxpz_update(self):
        try:
            response = requests.get("https://api.github.com/repos/m5kro/mkxp-z/commits/dev", timeout=1)
            response.raise_for_status()
            latest_commit = response.json()
            global latest_commit_sha
            latest_commit_sha = latest_commit['sha']
            logging.info("Latest commit SHA: %s", latest_commit_sha)
            return latest_commit_sha
        except requests.RequestException as e:
            logging.error("Failed to fetch latest commit SHA: %s", str(e))
            return last_commit_sha

    # NWJS version selector
    def update_version_selector(self):
        applications_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher")
        if os.path.exists(applications_dir):
            versions = [
                v for v in os.listdir(applications_dir) 
                if os.path.isdir(os.path.join(applications_dir, v)) and v.startswith("v")
            ]
            versions.sort(key=lambda x: list(map(int, re.findall(r'\d+', x))), reverse=True)

            self.version_selector.clear()
            self.version_selector.addItems(versions)
        self.update_select_button_state()

    def update_select_button_state(self):
        if self.check_nwjs_installed():
            self.select_button.setEnabled(True)
            self.select_button.setText("Select Game Folder")
        else:
            self.select_button.setEnabled(False)
            self.select_button.setText("NWJS not installed")

    def select_folder(self):
        folder_path = QFileDialog.getExistingDirectory(self, "Select Game Folder", self.last_selected_folder)
        if folder_path:
            self.last_selected_folder = folder_path
            logging.info("Selected folder path: %s", folder_path)
            
            if self.check_package_json(folder_path):
                logging.info("Package.json found in the selected folder.")
            elif self.check_game_ini(folder_path):
                logging.info("Game.ini file found in the folder.")
            elif self.check_RTP_RT(folder_path):
                logging.info("RTP_RT files found in the folder.")
            else:
                logging.error("No RPG Maker files found in the selected folder.")
                QMessageBox.critical(self, "Error", "No package.json, Game.ini, or RPG_RT files found in the selected folder.")
        self.save_settings()
        self.update_selected_folder_label()
        self.check_start_game_button()

    def update_selected_folder_label(self):
        if self.last_selected_folder:
            self.selected_folder_label.setText(f"Selected Folder: {self.truncate_path(self.last_selected_folder)}")
        else:
            self.selected_folder_label.setText("No folder selected")

    def truncate_path(self, path, max_length=50):
        if len(path) > max_length:
            return "..." + path[-(max_length - 3):]
        return path

    def truncate_filename(self, filename, max_length=30):
        if len(filename) > max_length:
            return filename[:max_length // 2] + "..." + filename[-(max_length // 2):]
        return filename

    def check_start_game_button(self):
        if self.last_selected_folder and self.version_selector.currentText():
            self.start_game_button.setEnabled(True)
            self.export_button.setEnabled(True)
        else:
            self.start_game_button.setEnabled(False)
            self.export_button.setEnabled(False)

    # Some MV and MZ games don't include a name inside package.json, so we patch it with a temporary name
    # This is a requirement for newer versions of NWJS to launch the game
    def check_package_json(self, folder_path):
        file_path = os.path.join(folder_path, 'package.json')
        if os.path.exists(file_path):
            with open(file_path, 'rb') as file:
                raw_data = file.read()
                result = chardet.detect(raw_data)
                encoding = result['encoding']

            with open(file_path, 'r', encoding=encoding) as file:
                data = json.load(file)

            if 'name' in data and data['name'].strip() == '':
                data['name'] = 'tempname'

            with open(file_path, 'w', encoding=encoding) as file:
                json.dump(data, file, indent=4)

            logging.info("JSON file updated successfully.")
            return True
        else:
            logging.error("File does not exist.")
            return False

    # Game.ini is a configuration file used by RPG Maker XP, VX, and VX Ace games
    def check_game_ini(self, folder_path):
        game_ini_path = os.path.join(folder_path, "Game.ini")
        return os.path.exists(game_ini_path)
    
    # Check for any files named RTP_RT to indicate it's RPG 2000 or RPG 2003
    def check_RTP_RT(self, folder_path):
        if os.path.exists(folder_path):
            for file in os.listdir(folder_path):
                full_path = os.path.join(folder_path, file)
                if os.path.isfile(full_path) and file.startswith("RPG_RT"):
                    return True
        else:
            logging.error(f"Folder does not exist: {folder_path}")    
        return False
    
    # This function checks the RTP value in the Game.ini file or RGSS DLL files to set the correct RTP version
    def get_rtp_value(self, folder_path):
        game_ini_path = os.path.join(folder_path, "Game.ini")
        try:
            with open(game_ini_path, 'r') as file:
                lines = file.readlines()
                for line in lines:
                    match = re.match(r"\s*rtp\s*=\s*(.*)", line, re.IGNORECASE)
                    if match:
                        logging.info(f"RTP value found.")
                        if match.group(1).strip().lower() == "standard":
                            return "Standard", 1
                        elif match.group(1).strip().lower() == "rpgvx":
                            return "RPGVX", 2
                        elif match.group(1).strip().lower() == "rpgvxace":
                            return "RPGVXace", 3
                        else:
                            logging.warning(f"Unknown RTP value found in Game.ini: {match.group(1)}")
                for line in lines:
                    match = re.match(r"\s*library\s*=\s*(.*)", line, re.IGNORECASE)
                    if match:
                        logging.info(f"Library value found.")
                        match = re.search(r"RGSS(\d+)", match.group(1), re.IGNORECASE)
                        if match:
                            rtp_value = int(match.group(1)[0])
                            if rtp_value == 1:
                                return "Standard", 1
                            elif rtp_value == 2:
                                return "RPGVX", 2
                            elif rtp_value == 3:
                                return "RPGVXace", 3
                            else:
                                logging.warning(f"Unknown RTP value found in Game.ini: {match.group(1)[0]}")
            logging.warning("RTP value not found in Game.ini, looking for dll files.")
        except FileNotFoundError:
            logging.error(f"Game.ini file not found in the folder: {folder_path}")
        
        try:
            for file in os.listdir(folder_path):
                if file.startswith("RGSS") and file.endswith(".dll"):
                    match = re.search(r"(\d+)", file)
                    if match:
                        logging.info(f"RGSS DLL file found.")
                        rtp_value = int(match.group(1)[0])
                        if rtp_value == 1:
                            return "Standard", 1
                        elif rtp_value == 2:
                            return "RPGVX", 2
                        elif rtp_value == 3:
                            return "RPGVXace", 3
                        else:
                            logging.warning(f"Unknown RTP value found in DLL file: {match.group(1)[0]}")
            logging.warning("No RGSS DLL files found in the game folder, checking System folder.")
        except FileNotFoundError as e:
            logging.error(f"Game folder error: {e}")

        system_path = os.path.join(folder_path, "System")
        try:
            for file in os.listdir(system_path):
                if file.startswith("RGSS") and file.endswith(".dll"):
                    match = re.search(r"(\d+)", file)
                    if match:
                        logging.info(f"RGSS DLL file found in System folder.")
                        rtp_value = int(match.group(1)[0])
                        if rtp_value == 1:
                            return "Standard", 1
                        elif rtp_value == 2:
                            return "RPGVX", 2
                        elif rtp_value == 3:
                            return "RPGVXace", 3
                        else:
                            logging.warning(f"Unknown RTP value found in System folder: {match.group(1)[0]}")
            logging.warning("No RGSS DLL files found in the System folder, assuming Standard RTP.")
        except FileNotFoundError as e:
            logging.warning(f"System folder not found in the folder: {e}")
            logging.info("Assuming Standard RTP.")

        # If no RTP value is found, default to Standard RTP (RPG Maker XP)
        return "Standard", 1

    # Some japanese RPG Maker games use game_en.exe to patch the game with the English version
    # They need to be extracted using evbunpack and the files copied to the game folder
    def check_and_unpack_game_en(self, folder_path):
        game_exe = None

        for root, _, files in os.walk(folder_path):
            for file in files:
                if file.lower() == "game_en.exe":
                    game_exe = os.path.join(root, file)
                    break
            if game_exe:
                break

        if game_exe:
            extracted_dir = os.path.join(folder_path, "extracted")
            unpack_files(game_exe, extracted_dir, False, False)

            for item in os.listdir(extracted_dir):
                s = os.path.join(extracted_dir, item)
                d = os.path.join(folder_path, item)
                if os.path.isdir(s):
                    shutil.copytree(s, d, dirs_exist_ok=True)
                else:
                    shutil.copy2(s, d)

            os.remove(game_exe)
            shutil.rmtree(extracted_dir)
            logging.info("game_en.exe extracted and original file removed.")
        else:
            logging.error("game_en.exe not found in the directory")

    def start_game(self):
        folder_path = self.last_selected_folder
        if not folder_path:
            logging.error("No folder selected.")
            QMessageBox.critical(self, "Error", "No folder selected.")
            return
        
        # Giving ownership of the game folder to the current user to bypass permission issues because apple can't let us have nice things
        logging.info("Setting you as owner of the game folder and permissions to 700 (You can read, write, execute).")
        try:
            subprocess.run(["chown", "-R", str(os.getuid()), folder_path], check=True)
            logging.info("Changed ownership of the folder to the current user.")
        except subprocess.CalledProcessError as e:
            logging.error("Failed to change ownership of the folder: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to change ownership of the folder: {str(e)}")
            return
        try:
            subprocess.run(["chmod", "-R", "700", folder_path], check=True)
            logging.info("Changed permissions of the folder to 700.")
        except subprocess.CalledProcessError as e:
            logging.error("Failed to change permissions of the folder: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to change permissions of the folder: {str(e)}")
            return

        if self.check_package_json(folder_path):
            logging.info("Launching game using NWJS.")

            if self.extract_checkbox.isChecked():
                self.check_and_unpack_game_en(folder_path)

            if self.optimize_space_checkbox.isChecked():
                self.optimize_space()

            if self.cheat_menu_checkbox.isChecked():
                self.add_cheat_menu(folder_path)
            else:
                self.remove_cheat_menu(folder_path)
            
            self.launch_nwjs_game(folder_path)
        elif self.check_game_ini(folder_path):
            logging.info("Launching game using MKXPZ.")
            self.launch_mkxpz_game(folder_path)
        elif self.check_RTP_RT(folder_path):
            logging.info("Launching game using EasyRPG.")
            self.launch_easyrpg_game(folder_path)
        else:
            logging.error("No Package.json, Game.ini, or RTP_RT files found in the folder.")
            QMessageBox.critical(self, "Error", "No valid game file (package.json, Game.ini, or RTP_RT) found in the selected folder.")

    def launch_nwjs_game(self, folder_path):
        selected_version = self.version_selector.currentText()
        if not selected_version:
            logging.error("No NWJS version selected.")
            QMessageBox.critical(self, "Error", "No NWJS version selected.")
            return

        nwjs_dir = os.path.expanduser(f"~/Library/Application Support/RPGM-Launcher/{selected_version}")
        nwjs_path = os.path.join(nwjs_dir, "nwjs.app/Contents/MacOS/nwjs")

        # Older versions of NWJS don't have native arm so rosetta is required
        run_with_rosetta_file = os.path.join(nwjs_dir, "run-with-rosetta")
        if os.path.exists(run_with_rosetta_file):
            subprocess.Popen(["arch", "-x86_64", nwjs_path, folder_path])
        else:
            subprocess.Popen([nwjs_path, folder_path])
        logging.info("Game launched using NWJS version %s.", selected_version)

    def launch_mkxpz_game(self, folder_path):
        if self.check_mkxpz_installed(warn=False):
            global last_commit_sha
            logging.info("Checking for MKXP-Z updates.")
            logging.info("Last commit SHA: %s", last_commit_sha)
            if last_commit_sha != self.check_mkxpz_update():
                update_response = QMessageBox.question(self, "MKXP-Z Update Available",
                                                       "A new update is available for MKXP-Z. Would you like to update now?",
                                                       QMessageBox.Yes | QMessageBox.No)
                if update_response == QMessageBox.Yes:
                    self.install_mkxpz()
                    
        if not self.check_mkxpz_installed() or not self.check_RTP_installed() or not self.check_soundfont_installed() or not self.check_kawariki_installed():
            return

        enabled_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/enabled-mkxpz-settings.json")
        advanced_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/mkxpz-advanced.json")

        # Apply default settings if the files do not exist
        if not os.path.exists(enabled_settings_file):
            with open(enabled_settings_file, 'w') as file:
                json.dump(default_enabled_settings, file, indent=4)
            logging.info("Created enabled-mkxpz-settings.json with default values.")

        if not os.path.exists(advanced_settings_file):
            with open(advanced_settings_file, 'w') as file:
                json.dump(default_advanced_settings, file, indent=4)
            logging.info("Created mkxpz-advanced.json with default values.")

        # Load the advanced settings
        with open(enabled_settings_file, 'r') as file:
            enabled_settings = json.load(file)

        with open(advanced_settings_file, 'r') as file:
            advanced_settings = json.load(file)

        mkxpz_json_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/Z-universal.app/Contents/Game/mkxp.json")
        soundfont_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/GMGSx.SF2")
        rtp_value, rgss_version = self.get_rtp_value(folder_path)
        rtp_path = os.path.join(os.path.expanduser("~/Library/Application Support/RPGM-Launcher/RTP"), rtp_value)
        kawariki_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/kawariki/preload.rb")

        if os.path.exists(mkxpz_json_path):
            with open(mkxpz_json_path, 'rb') as file:
                raw_data = file.read()
                result = chardet.detect(raw_data)
                encoding = result['encoding']
            with open(mkxpz_json_path, 'r', encoding=encoding) as file:
                mkxp_config = json.load(file)
        else:
            mkxp_config = {}

        # Update the configuration with enabled settings
        for key, enabled in enabled_settings.items():
            if enabled in ["True", "Force"]:
                mkxp_config[key] = advanced_settings.get(key, mkxp_config.get(key))
                if key == "gameFolder" and not advanced_settings[key]:
                    mkxp_config["gameFolder"] = folder_path
                elif key == "RTP" and not advanced_settings[key]:
                    mkxp_config["RTP"] = [rtp_path]
                elif key == "midiSoundFont" and not advanced_settings[key]:
                    mkxp_config["midiSoundFont"] = soundfont_path
                elif key == "preloadScript" and not advanced_settings[key]:
                    mkxp_config["preloadScript"] = [kawariki_path]
                elif key == "rgssVersion" and advanced_settings[key] == 0:
                    mkxp_config["rgssVersion"] = rgss_version
            else:
                mkxp_config.pop(key, None)

        # Save the updated configuration and launch the game
        try:
            with open(mkxpz_json_path, 'rb') as file:
                raw_data = file.read()
                result = chardet.detect(raw_data)
                encoding = result['encoding']
            with open(mkxpz_json_path, 'w', encoding=encoding) as file:
                json.dump(mkxp_config, file, indent=4)
            logging.info("Updated mkxp.json with enabled settings.")
            logging.info("Launching with gameFolder: %s, RTP: %s, midiSoundFont: %s, preloadScript: %s, rgssVersion: %s", mkxp_config.get("gameFolder"), mkxp_config.get("RTP"), mkxp_config.get("midiSoundFont"), mkxp_config.get("preloadScript"), mkxp_config.get("rgssVersion"))
        except Exception as e:
            logging.error("Failed to update mkxp.json: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to update mkxp.json: {str(e)}")
            return

        mkxpz_executable_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/Z-universal.app/Contents/MacOS/Z-universal")
        try:
            subprocess.Popen([mkxpz_executable_path, folder_path])
            logging.info("MKXPZ game launched from folder: %s", folder_path)
        except Exception as e:
            logging.error("Failed to launch MKXPZ: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to launch MKXPZ: {str(e)}")
    
    def launch_easyrpg_game(self, folder_path):
        easyrpg_executable_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/EasyRPG Player.app/Contents/MacOS/EasyRPG Player")
        if not self.check_easyrpg_installed():
            logging.error("EasyRPG Player not found at %s", easyrpg_executable_path)
            QMessageBox.critical(self, "Error", "EasyRPG Player not found. Please install it first.")
            return
        
        try:
            # EasyRPG only has an intel version, so Rosetta is required on Apple Silicon Macs
            arch = "arm64" if platform.machine() == "arm64" else "x64"
            if arch == "arm64":
                subprocess.Popen(["arch", "-x86_64", easyrpg_executable_path, "--window", "--project-path", folder_path])
                logging.info("EasyRPG game launched from folder: %s using Rosetta", folder_path)
            else:
                subprocess.Popen([easyrpg_executable_path, "--window", "--project-path", folder_path])
                logging.info("EasyRPG game launched from folder: %s", folder_path)
        except Exception as e:
            logging.error("Failed to launch EasyRPG: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to launch EasyRPG: {str(e)}")

    def export_standalone_app(self):
        app_name, ok = self.get_app_name()
        if not ok:
            return

        destination_folder = QFileDialog.getExistingDirectory(self, "Select Destination Folder")
        if not destination_folder:
            return

        folder_path = self.last_selected_folder

        try:
            if self.check_package_json(folder_path):
                # NWJS export process
                selected_version = self.version_selector.currentText()
                if not selected_version:
                    logging.error("No NWJS version selected.")
                    QMessageBox.critical(self, "Error", "No NWJS version selected.")
                    return

                nwjs_dir = os.path.expanduser(f"~/Library/Application Support/RPGM-Launcher/{selected_version}")
                nwjs_app_src = os.path.join(nwjs_dir, "nwjs.app")
                nwjs_app_dst = os.path.join(destination_folder, app_name + ".app")

                if self.extract_checkbox.isChecked():
                    self.check_and_unpack_game_en(self.last_selected_folder)

                if self.cheat_menu_checkbox.isChecked():
                    self.add_cheat_menu(self.last_selected_folder)

                if self.optimize_space_checkbox.isChecked():
                    self.optimize_space()

                total_files = sum([len(files) for _, _, files in os.walk(self.last_selected_folder)])
                progress_dialog = QProgressDialog("Exporting standalone app...", "Cancel", 0, total_files, self)
                progress_dialog.setWindowModality(Qt.WindowModal)
                progress_dialog.setMinimumDuration(0)
                progress_dialog.show()

                shutil.copytree(nwjs_app_src, nwjs_app_dst)

                app_nw_dir = os.path.join(nwjs_app_dst, "Contents", "Resources", "app.nw")
                os.makedirs(app_nw_dir, exist_ok=True)

                current_file_count = 0
                for root, _, files in os.walk(self.last_selected_folder):
                    for file in files:
                        s = os.path.join(root, file)
                        d = os.path.join(app_nw_dir, os.path.relpath(s, self.last_selected_folder))
                        os.makedirs(os.path.dirname(d), exist_ok=True)
                        shutil.copy2(s, d)
                        current_file_count += 1
                        progress_dialog.setValue(current_file_count)
                        progress_dialog.setLabelText(f"Copying over: {self.truncate_filename(file)}")
                        QApplication.processEvents()
                        if progress_dialog.wasCanceled():
                            logging.info("Export canceled by user.")
                            QMessageBox.information(self, "Export Canceled", "Export operation was canceled.")
                            return

                progress_dialog.close()
                QMessageBox.information(self, "Export Complete", "Standalone app exported successfully.")
                logging.info("Standalone app exported successfully to %s", destination_folder)

                QMessageBox.warning(self, "First Launch Warning", "Due to MacOS permissions, the first launch of the standalone app may stall or crash. To fix, simply force quit the app and reopen it.")
                
            elif self.check_game_ini(folder_path):
                if not self.check_mkxpz_installed() or not self.check_RTP_installed() or not self.check_soundfont_installed():
                    return

                enabled_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/enabled-mkxpz-settings.json")
                advanced_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/mkxpz-advanced.json")

                if not os.path.exists(enabled_settings_file):
                    with open(enabled_settings_file, 'w') as file:
                        json.dump(default_enabled_settings, file, indent=4)
                    logging.info("Created enabled-mkxpz-settings.json with default values.")

                if not os.path.exists(advanced_settings_file):
                    with open(advanced_settings_file, 'w') as file:
                        json.dump(default_advanced_settings, file, indent=4)
                    logging.info("Created mkxpz-advanced.json with default values.")

                with open(enabled_settings_file, 'r') as file:
                    enabled_settings = json.load(file)

                with open(advanced_settings_file, 'r') as file:
                    advanced_settings = json.load(file)

                mkxpz_json_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/Z-universal.app/Contents/Game/mkxp.json")
                soundfont_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/GMGSx.SF2")
                rtp_value, rgss_version = self.get_rtp_value(folder_path)
                rtp_path = os.path.join(os.path.expanduser("~/Library/Application Support/RPGM-Launcher/RTP"), rtp_value)
                kawariki_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/kawariki")

                if os.path.exists(mkxpz_json_path):
                    with open(mkxpz_json_path, 'rb') as file:
                        raw_data = file.read()
                        result = chardet.detect(raw_data)
                        encoding = result['encoding']
                    with open(mkxpz_json_path, 'r', encoding=encoding) as file:
                        mkxp_config = json.load(file)
                else:
                    mkxp_config = {}

                for key, enabled in enabled_settings.items():
                    if enabled in ["True", "Force"]:
                        mkxp_config[key] = advanced_settings.get(key, mkxp_config.get(key))
                        if key == "gameFolder" and not advanced_settings[key]:
                            mkxp_config["gameFolder"] = folder_path
                        elif key == "RTP" and not advanced_settings[key]:
                            mkxp_config["RTP"] = [rtp_path]
                        elif key == "midiSoundFont" and not advanced_settings[key]:
                            mkxp_config["midiSoundFont"] = soundfont_path
                        elif key == "preloadScript" and not advanced_settings[key]:
                            mkxp_config["preloadScript"] = [kawariki_path]
                        elif key == "rgssVersion" and advanced_settings[key] == 0:
                            mkxp_config["rgssVersion"] = rgss_version
                    else:
                        mkxp_config.pop(key, None)

                try:
                    with open(mkxpz_json_path, 'rb') as file:
                        raw_data = file.read()
                        result = chardet.detect(raw_data)
                        encoding = result['encoding']
                    with open(mkxpz_json_path, 'w', encoding=encoding) as file:
                        json.dump(mkxp_config, file, indent=4)
                    logging.info("Updated mkxp.json with enabled settings.")
                    logging.info("Setting: %s, RTP: %s, midiSoundFont: %s, preloadScript: %s, rgssVersion: %s", mkxp_config.get("gameFolder"), mkxp_config.get("RTP"), mkxp_config.get("midiSoundFont"), mkxp_config.get("preloadScript"), mkxp_config.get("rgssVersion"))
                except Exception as e:
                    logging.error("Failed to update mkxp.json: %s", str(e))
                    QMessageBox.critical(self, "Error", f"Failed to update mkxp.json: {str(e)}")
                    return

                # MKXPZ export process
                mkxpz_app_src = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/Z-universal.app")
                mkxpz_app_dst = os.path.join(destination_folder, app_name + ".app")

                shutil.copytree(mkxpz_app_src, mkxpz_app_dst)

                game_dir = os.path.join(mkxpz_app_dst, "Contents", "Game")

                total_files = sum([len(files) for _, _, files in os.walk(folder_path)])

                progress_dialog = QProgressDialog("Exporting standalone app...", "Cancel", 0, total_files, self)
                progress_dialog.setWindowModality(Qt.WindowModal)
                progress_dialog.setMinimumDuration(0)
                progress_dialog.show()

                current_file_count = 0
                for root, _, files in os.walk(folder_path):
                    for file in files:
                        s = os.path.join(root, file)
                        d = os.path.join(game_dir, os.path.relpath(s, folder_path))
                        os.makedirs(os.path.dirname(d), exist_ok=True)
                        shutil.copy2(s, d)
                        current_file_count += 1
                        progress_dialog.setValue(current_file_count)
                        progress_dialog.setLabelText(f"Copying over: {self.truncate_filename(file)}")
                        QApplication.processEvents()
                        if progress_dialog.wasCanceled():
                            logging.info("Export canceled by user.")
                            QMessageBox.information(self, "Export Canceled", "Export operation was canceled.")
                            return

                rtp_value, rgss_version = self.get_rtp_value(folder_path)
                rtp_src = os.path.join(os.path.expanduser("~/Library/Application Support/RPGM-Launcher/RTP"), rtp_value)
                rtp_dst = os.path.join(game_dir, rtp_value)
                shutil.copytree(rtp_src, rtp_dst, dirs_exist_ok=True)

                soundfont_src = os.path.expanduser(mkxp_config["midiSoundFont"])
                soundfont_dst = os.path.join(game_dir, "GMGSx.SF2")
                shutil.copy2(soundfont_src, soundfont_dst)

                kawariki_src = os.path.expanduser(mkxp_config["preloadScript"][0])
                kawariki_dst = os.path.join(game_dir, "kawariki")
                shutil.copytree(kawariki_src, kawariki_dst, dirs_exist_ok=True)
                

                mkxp_json_path = os.path.join(game_dir, "mkxp.json")
                if os.path.exists(mkxp_json_path):
                    try:
                        with open(mkxp_json_path, 'rb') as file:
                            raw_data = file.read()
                            result = chardet.detect(raw_data)
                            encoding = result['encoding']

                        with open(mkxp_json_path, 'r', encoding=encoding) as file:
                            mkxp_config = json.load(file)

                        mkxp_config["gameFolder"] = "./"
                        mkxp_config["RTP"] = [f"./{rtp_value}"]
                        mkxp_config["midiSoundFont"] = f"./{os.path.basename(soundfont_dst)}"
                        mkxp_config["preloadScript"] = [f"./{os.path.join("kawariki", "preload.rb")}"]

                        with open(mkxp_json_path, 'w', encoding=encoding) as file:
                            json.dump(mkxp_config, file, indent=4)
                        
                        logging.info("Updated mkxp.json with gameFolder: './' and RTP: './%s'", rtp_value)

                    except Exception as e:
                        logging.error("Failed to update mkxp.json: %s", str(e))
                        QMessageBox.critical(self, "Error", f"Failed to update mkxp.json: {str(e)}")
                        return

                progress_dialog.close()
                QMessageBox.information(self, "Export Complete", "MKXPZ standalone app exported successfully.")
                logging.info("MKXPZ standalone app exported successfully to %s", destination_folder)
            
            elif self.check_RTP_RT(folder_path):
                # EasyRPG export process
                easyrpg_app_src = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/EasyRPG Player.app")
                easyrpg_app_dst = os.path.join(destination_folder, app_name + ".app")

                os.mkdir(easyrpg_app_dst)

                game_dir = os.path.join(easyrpg_app_dst, "game")

                total_files = sum([len(files) for _, _, files in os.walk(folder_path)])

                progress_dialog = QProgressDialog("Exporting standalone app...", "Cancel", 0, total_files, self)
                progress_dialog.setWindowModality(Qt.WindowModal)
                progress_dialog.setMinimumDuration(0)
                progress_dialog.show()

                current_file_count = 0
                for root, _, files in os.walk(folder_path):
                    for file in files:
                        s = os.path.join(root, file)
                        d = os.path.join(game_dir, os.path.relpath(s, folder_path))
                        os.makedirs(os.path.dirname(d), exist_ok=True)
                        shutil.copy2(s, d)
                        current_file_count += 1
                        progress_dialog.setValue(current_file_count)
                        progress_dialog.setLabelText(f"Copying over: {self.truncate_filename(file)}")
                        QApplication.processEvents()
                        if progress_dialog.wasCanceled():
                            logging.info("Export canceled by user.")
                            QMessageBox.information(self, "Export Canceled", "Export operation was canceled.")
                            return

                # Script required for standlone app operation
                # Also apple why did you have to make this so awful. Why do I need to copy the entire app just to keep it executable?
                progress_dialog.setLabelText("Copying support files...")
                logging.info("Copying support files for EasyRPG standalone app...")
                shutil.copy(os.path.join(os.path.dirname(__file__), 'EasyRPG-Standalone'), os.path.join(easyrpg_app_dst))
                shutil.copytree(easyrpg_app_src, os.path.join(easyrpg_app_dst, "EasyRPG Player.app"), dirs_exist_ok=True)
                logging.info("Support files copied successfully. Moving executables...")
                shutil.move(os.path.join(easyrpg_app_dst, "EasyRPG-Standalone"), os.path.join(easyrpg_app_dst, app_name))
                logging.info("Executables moved successfully. Setting permissions...")
                subprocess.run(["chmod", "+x", os.path.join(easyrpg_app_dst, app_name)], check=True)
                subprocess.run(["chmod", "+x", os.path.join(easyrpg_app_dst, "EasyRPG Player.app", "Contents", "MacOS", "EasyRPG Player")], check=True)

                progress_dialog.close()
                QMessageBox.information(self, "Export Complete", "EasyRPG standalone app exported successfully.")
                logging.info("EasyRPG standalone app exported successfully to %s", destination_folder)

            else:
                logging.error("No valid game file (package.json, Game.ini, or RTP_RT) found in the selected folder.")
                QMessageBox.critical(self, "Error", "No valid game file (package.json, Game.ini, or RTP_RT) found in the selected folder.")

        except Exception as e:
            logging.error("Error exporting standalone app: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to export standalone app: {str(e)}")

    def get_app_name(self):
        dialog = QDialog(self)
        dialog.setWindowTitle("Enter Application Name")
        layout = QVBoxLayout(dialog)

        label = QLabel("Enter a name for the new application:", dialog)
        layout.addWidget(label)

        line_edit = QLineEdit(dialog)
        line_edit.setPlaceholderText("Game Name")
        layout.addWidget(line_edit)

        buttons = QDialogButtonBox(QDialogButtonBox.Ok | QDialogButtonBox.Cancel, dialog)
        buttons.accepted.connect(dialog.accept)
        buttons.rejected.connect(dialog.reject)
        layout.addWidget(buttons)

        result = dialog.exec()
        app_name = line_edit.text().strip() if line_edit.text().strip() else "Game Name"
        return app_name, result == QDialog.Accepted

    # Install NWJS versions, multiple versions can be installed
    def install_nwjs(self):
        URL = "https://nwjs.io/versions"
        BASE_DIR = os.path.expanduser("~/Library/Application Support/RPGM-Launcher")

        def download_and_install(version, arch, use_rosetta=False):
            url = f"https://dl.nwjs.io/{version}/nwjs-sdk-{version}-osx-{arch}.zip"
            tmp_file = "/tmp/nwjs.zip"

            logging.info("Downloading and installing version %s for %s architecture...", version, arch)
            response = requests.get(url, stream=True)
            if response.status_code != 200:
                logging.error("Failed to download version %s for %s.", version, arch)
                return

            total_size = int(response.headers.get('content-length', 0))
            downloaded_size = 0
            chunk_size = 1024  # 1KB

            progress_dialog = QProgressDialog("Downloading NWJS...", "Cancel", 0, total_size, self)
            progress_dialog.setWindowModality(Qt.WindowModal)
            progress_dialog.setMinimumDuration(0)
            progress_dialog.show()

            start_time = QDateTime.currentDateTime()
            canceled = False

            with open(tmp_file, "wb") as f:
                for chunk in response.iter_content(chunk_size):
                    if chunk:
                        f.write(chunk)
                        downloaded_size += len(chunk)
                        progress_dialog.setValue(downloaded_size)

                        elapsed_time = start_time.msecsTo(QDateTime.currentDateTime()) / 1000

                        if elapsed_time > 0:
                            download_speed = downloaded_size / (1024 * 1024) / elapsed_time
                        else:
                            download_speed = 0

                        progress_dialog.setLabelText(f"Downloaded: {downloaded_size / (1024 * 1024):.2f} MB of {total_size / (1024 * 1024):.2f} MB\n"
                                                     f"Speed: {download_speed:.2f} MB/s")

                        if progress_dialog.wasCanceled():
                            canceled = True
                            logging.info("Download canceled by user.")
                            break

            if canceled:
                os.remove(tmp_file)
                QMessageBox.information(self, "NWJS Installation", "NWJS Installation Canceled.")
                return

            subprocess.run(["unzip", "-q", tmp_file, "-d", "/tmp"])

            target_dir = os.path.join(BASE_DIR, version)
            if os.path.exists(target_dir):
                shutil.rmtree(target_dir)
            
            os.makedirs(target_dir, exist_ok=True)
            shutil.move(f"/tmp/nwjs-sdk-{version}-osx-{arch}/nwjs.app", target_dir)
            logging.info("Version %s installed successfully at %s", version, target_dir)

            if use_rosetta:
                with open(os.path.join(target_dir, "run-with-rosetta"), "w") as f:
                    f.write("This file indicates NWJS should be run with Rosetta 2.")

            os.remove(tmp_file)
            shutil.rmtree(f"/tmp/nwjs-sdk-{version}-osx-{arch}")

            progress_dialog.close()
            QMessageBox.information(self, "Install NWJS Version", "NWJS installation completed successfully.")

        arch = "arm64" if platform.machine() == "arm64" else "x64"
        logging.info("Querying available versions...")
        response = requests.get(URL)
        if response.status_code != 200:
            logging.error("Failed to query NWJS versions.")
            return

        data = response.json()
        versions = {v["version"]: v for v in data["versions"]}
        
        version, ok = self.show_version_selection_dialog(versions.keys())
        if ok and version:
            version_info = versions[version]
            use_rosetta = False
            if arch == "arm64" and "osx-arm64" not in version_info["files"]:
                use_rosetta = self.show_rosetta_warning()
                if not use_rosetta:
                    return
                arch = "x64"
            download_and_install(version, arch, use_rosetta)
            self.save_settings()
            self.update_version_selector()
            self.load_settings()
            self.check_start_game_button()

    # Install MKXP-Z, updates will be checked automatically
    def install_mkxpz(self):
        # The URL for the MKXP-Z universal build is under my fork because it needs to be built manually
        URL = "https://github.com/m5kro/mkxp-z/releases/download/launcher/Z-universal.zip"
        zip_file_path = "/tmp/Z-universal.zip"
        target_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/")

        try:
            response = requests.get(URL, stream=True)
            if response.status_code != 200:
                logging.error("Failed to download MKXPZ.")
                QMessageBox.critical(self, "Error", "Failed to download MKXPZ.")
                return

            total_size = int(response.headers.get('content-length', 0))
            downloaded_size = 0
            chunk_size = 1024  # 1KB

            progress_dialog = QProgressDialog("Downloading MKXPZ...", "Cancel", 0, total_size, self)
            progress_dialog.setWindowModality(Qt.WindowModal)
            progress_dialog.setMinimumDuration(0)
            progress_dialog.show()

            start_time = QDateTime.currentDateTime()
            canceled = False

            with open(zip_file_path, "wb") as f:
                for chunk in response.iter_content(chunk_size):
                    if chunk:
                        f.write(chunk)
                        downloaded_size += len(chunk)
                        progress_dialog.setValue(downloaded_size)

                        elapsed_time = start_time.msecsTo(QDateTime.currentDateTime()) / 1000

                        if elapsed_time > 0:
                            download_speed = downloaded_size / (1024 * 1024) / elapsed_time
                        else:
                            download_speed = 0

                        progress_dialog.setLabelText(f"Downloaded: {downloaded_size / (1024 * 1024):.2f} MB of {total_size / (1024 * 1024):.2f} MB\n"
                                                    f"Speed: {download_speed:.2f} MB/s")

                        if progress_dialog.wasCanceled():
                            canceled = True
                            logging.info("Download canceled by user.")
                            break

            if canceled:
                os.remove(zip_file_path)
                QMessageBox.information(self, "MKXPZ Installation", "MKXPZ installation canceled.")
                return

            with zipfile.ZipFile(zip_file_path, 'r') as zip_ref:
                zip_ref.extractall(target_dir)
            logging.info("MKXPZ extracted successfully to %s", target_dir)

            os.remove(zip_file_path)

            mkxpz_executable_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/Z-universal.app/Contents/MacOS/Z-universal")
            try:
                subprocess.run(["chmod", "+x", mkxpz_executable_path], check=True)
                logging.info("Set executable permissions for %s", mkxpz_executable_path)
            except subprocess.CalledProcessError as e:
                logging.error("Failed to set executable permissions: %s", str(e))
                QMessageBox.critical(self, "Error", f"Failed to set executable permissions: {str(e)}")
                return

            progress_dialog.close()
            global last_commit_sha
            global latest_commit_sha
            last_commit_sha = latest_commit_sha
            QMessageBox.information(self, "MKXPZ Installation", "MKXPZ installed successfully.")
            

        except Exception as e:
            logging.error("Error installing MKXPZ: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to install MKXPZ: {str(e)}")
    
    # Install RTP (Run Time Package) for RPG Maker games
    def install_RTP(self):
        URL = "https://github.com/m5kro/mkxp-z/releases/download/launcher/RTP.zip"
        zip_file_path = "/tmp/RTP.zip"
        target_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher")

        try:
            response = requests.get(URL, stream=True)
            if response.status_code != 200:
                logging.error("Failed to download RTP.")
                QMessageBox.critical(self, "Error", "Failed to download RTP.")
                return

            total_size = int(response.headers.get('content-length', 0))
            downloaded_size = 0
            chunk_size = 1024  # 1KB

            progress_dialog = QProgressDialog("Downloading RTP...", "Cancel", 0, total_size, self)
            progress_dialog.setWindowModality(Qt.WindowModal)
            progress_dialog.setMinimumDuration(0)
            progress_dialog.show()

            start_time = QDateTime.currentDateTime()
            canceled = False

            with open(zip_file_path, "wb") as f:
                for chunk in response.iter_content(chunk_size):
                    if chunk:
                        f.write(chunk)
                        downloaded_size += len(chunk)
                        progress_dialog.setValue(downloaded_size)

                        elapsed_time = start_time.msecsTo(QDateTime.currentDateTime()) / 1000

                        if elapsed_time > 0:
                            download_speed = downloaded_size / (1024 * 1024) / elapsed_time
                        else:
                            download_speed = 0

                        progress_dialog.setLabelText(f"Downloaded: {downloaded_size / (1024 * 1024):.2f} MB of {total_size / (1024 * 1024):.2f} MB\n"
                                                    f"Speed: {download_speed:.2f} MB/s")

                        if progress_dialog.wasCanceled():
                            canceled = True
                            logging.info("Download canceled by user.")
                            break

            if canceled:
                os.remove(zip_file_path)
                QMessageBox.information(self, "RTP Installation", "RTP installation canceled.")
                return

            with zipfile.ZipFile(zip_file_path, 'r') as zip_ref:
                zip_ref.extractall(target_dir)
            logging.info("RTP extracted successfully to %s", target_dir)

            os.remove(zip_file_path)
            progress_dialog.close()
            QMessageBox.information(self, "RTP Installation", "RTP installed successfully.")
            

        except Exception as e:
            logging.error("Error installing RTP: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to install RTP: {str(e)}")

    # Install soundfont for RPG Maker games
    def download_soundfont(self):
        URL = "https://musical-artifacts.com/artifacts/841/GMGSx.SF2"
        soundfont_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/GMGSx.SF2")

        if not os.path.exists(soundfont_path):
            try:
                response = requests.get(URL, stream=True)
                total_size = int(response.headers.get('content-length', 0))
                downloaded_size = 0
                chunk_size = 1024  # 1KB

                # Setup a progress dialog
                progress_dialog = QProgressDialog("Downloading GMGSx.SF2 Soundfont...", "Cancel", 0, total_size, self)
                progress_dialog.setWindowModality(Qt.WindowModal)
                progress_dialog.setMinimumDuration(0)
                progress_dialog.show()

                start_time = QDateTime.currentDateTime()
                canceled = False

                with open(soundfont_path, "wb") as file:
                    for chunk in response.iter_content(chunk_size):
                        if chunk:
                            file.write(chunk)
                            downloaded_size += len(chunk)

                            # Update progress dialog
                            progress_dialog.setValue(downloaded_size)
                            elapsed_time = start_time.msecsTo(QDateTime.currentDateTime()) / 1000

                            # Display download speed and progress
                            download_speed = downloaded_size / (1024 * 1024) / elapsed_time if elapsed_time > 0 else 0
                            progress_dialog.setLabelText(
                                f"Downloaded: {downloaded_size / (1024 * 1024):.2f} MB of {total_size / (1024 * 1024):.2f} MB\n"
                                f"Speed: {download_speed:.2f} MB/s"
                            )

                            # Check if the user has canceled the download
                            if progress_dialog.wasCanceled():
                                canceled = True
                                logging.info("Download canceled by user.")
                                break

                progress_dialog.close()

                if canceled:
                    os.remove(soundfont_path)
                    QMessageBox.information(self, "Soundfont Installation", "Soundfont download canceled.")
                    return
                else:
                    logging.info("Soundfont downloaded successfully to %s", soundfont_path)
                    QMessageBox.information(self, "Soundfont Installation", "Soundfont GMGSx.SF2 downloaded successfully.")

            except Exception as e:
                logging.error("Error downloading GMGSx.SF2: %s", str(e))
                QMessageBox.critical(self, "Error", f"Failed to download GMGSx.SF2: {str(e)}")
        else:
            logging.info("Soundfont GMGSx.SF2 already exists at %s", soundfont_path)
    
    # Install Kawariki patches for RPG Maker games
    def download_kawariki_patches(self):
        URL = "https://github.com/m5kro/mkxp-z/releases/download/launcher/kawariki.zip"
        zip_file_path = "/tmp/kawariki.zip"
        target_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/")

        try:
            response = requests.get(URL, stream=True)
            if response.status_code != 200:
                logging.error("Failed to download Kawariki patches.")
                QMessageBox.critical(self, "Error", "Failed to download Kawariki patches.")
                return

            total_size = int(response.headers.get('content-length', 0))
            downloaded_size = 0
            chunk_size = 1024  # 1KB

            progress_dialog = QProgressDialog("Downloading Kawariki patches...", "Cancel", 0, total_size, self)
            progress_dialog.setWindowModality(Qt.WindowModal)
            progress_dialog.setMinimumDuration(0)
            progress_dialog.show()

            start_time = QDateTime.currentDateTime()
            canceled = False

            with open(zip_file_path, "wb") as f:
                for chunk in response.iter_content(chunk_size):
                    if chunk:
                        f.write(chunk)
                        downloaded_size += len(chunk)
                        progress_dialog.setValue(downloaded_size)

                        elapsed_time = start_time.msecsTo(QDateTime.currentDateTime()) / 1000

                        if elapsed_time > 0:
                            download_speed = downloaded_size / (1024 * 1024) / elapsed_time
                        else:
                            download_speed = 0

                        progress_dialog.setLabelText(f"Downloaded: {downloaded_size / (1024 * 1024):.2f} MB of {total_size / (1024 * 1024):.2f} MB\n"
                                                        f"Speed: {download_speed:.2f} MB/s")

                        if progress_dialog.wasCanceled():
                            canceled = True
                            logging.info("Download canceled by user.")
                            break

            if canceled:
                os.remove(zip_file_path)
                QMessageBox.information(self, "Kawariki Patches Installation", "Kawariki patches installation canceled.")
                return

            with zipfile.ZipFile(zip_file_path, 'r') as zip_ref:
                zip_ref.extractall(target_dir)
            logging.info("Kawariki patches extracted successfully to %s", target_dir)

            os.remove(zip_file_path)
            progress_dialog.close()
            QMessageBox.information(self, "Kawariki Patches Installation", "Kawariki patches installed successfully.")

        except Exception as e:
            logging.error("Error installing Kawariki patches: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to install Kawariki patches: {str(e)}")
    
    # Install EasyRPG Player for RPG Maker 2000/2003 games
    def install_easyrpg(self):
        URL="https://ci.easyrpg.org/downloads/macos/EasyRPG-Player-macos.app.zip"
        zip_file_path = "/tmp/EasyRPG-Player-macos.app.zip"
        target_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/")
        try:
            response = requests.get(URL, stream=True)
            if response.status_code != 200:
                logging.error("Failed to download EasyRPG Player.")
                QMessageBox.critical(self, "Error", "Failed to download EasyRPG Player.")
                return

            total_size = int(response.headers.get('content-length', 0))
            downloaded_size = 0
            chunk_size = 1024  # 1KB

            progress_dialog = QProgressDialog("Downloading EasyRPG Player...", "Cancel", 0, total_size, self)
            progress_dialog.setWindowModality(Qt.WindowModal)
            progress_dialog.setMinimumDuration(0)
            progress_dialog.show()

            start_time = QDateTime.currentDateTime()
            canceled = False

            with open(zip_file_path, "wb") as f:
                for chunk in response.iter_content(chunk_size):
                    if chunk:
                        f.write(chunk)
                        downloaded_size += len(chunk)
                        progress_dialog.setValue(downloaded_size)

                        elapsed_time = start_time.msecsTo(QDateTime.currentDateTime()) / 1000

                        if elapsed_time > 0:
                            download_speed = downloaded_size / (1024 * 1024) / elapsed_time
                        else:
                            download_speed = 0

                        progress_dialog.setLabelText(f"Downloaded: {downloaded_size / (1024 * 1024):.2f} MB of {total_size / (1024 * 1024):.2f} MB\n"
                                                     f"Speed: {download_speed:.2f} MB/s")

                        if progress_dialog.wasCanceled():
                            canceled = True
                            logging.info("Download canceled by user.")
                            break

            if canceled:
                os.remove(zip_file_path)
                QMessageBox.information(self, "EasyRPG Player Installation", "EasyRPG Player installation canceled.")
                return

            with zipfile.ZipFile(zip_file_path, 'r') as zip_ref:
                zip_ref.extractall(os.path.dirname(target_dir))
            logging.info("EasyRPG Player extracted successfully to %s", target_dir)

            os.remove(zip_file_path)
            easyrpg_executable_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/EasyRPG Player.app/Contents/MacOS/EasyRPG Player")
            try:
                subprocess.run(["chmod", "+x", easyrpg_executable_path], check=True)
                logging.info("Set executable permissions for %s", easyrpg_executable_path)
            except subprocess.CalledProcessError as e:
                logging.error("Failed to set executable permissions: %s", str(e))
                QMessageBox.critical(self, "Error", f"Failed to set executable permissions: {str(e)}")
                return

            progress_dialog.close()
            QMessageBox.information(self, "EasyRPG Player Installation", "EasyRPG Player installed successfully.")
        except Exception as e:
            logging.error("Error installing EasyRPG Player: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to install EasyRPG Player: {str(e)}")

    def show_version_selection_dialog(self, versions):
        dialog = QDialog(self)
        dialog.setWindowTitle("Select NWJS Version")
        layout = QFormLayout(dialog)
        
        combo_box = QComboBox(dialog)
        combo_box.addItems(versions)
        layout.addRow("Version:", combo_box)

        buttons = QDialogButtonBox(QDialogButtonBox.Ok | QDialogButtonBox.Cancel, dialog)
        buttons.accepted.connect(dialog.accept)
        buttons.rejected.connect(dialog.reject)
        layout.addRow(buttons)

        result = dialog.exec()
        return combo_box.currentText(), result == QDialog.Accepted

    def show_rosetta_warning(self):
        dialog = QDialog(self)
        dialog.setWindowTitle("Rosetta Warning")
        layout = QVBoxLayout(dialog)

        label = QLabel("An Apple Silicon / ARM64 build of this version of NWJS is unavailable. Would you like to use the Intel / x64 version with Rosetta?", dialog)
        layout.addWidget(label)

        buttons = QDialogButtonBox(QDialogButtonBox.Yes | QDialogButtonBox.No, dialog)
        buttons.accepted.connect(dialog.accept)
        buttons.rejected.connect(dialog.reject)
        layout.addWidget(buttons)

        result = dialog.exec()
        return result == QDialog.Accepted

    def uninstall_nwjs(self):
        applications_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher")
        # Only keep the directories that are actually versions (starting with "v")

        versions = [v for v in os.listdir(applications_dir) if os.path.isdir(os.path.join(applications_dir, v)) and v.startswith("v")]

        version, ok = self.show_version_selection_dialog(versions)
        if ok and version:
            target_dir = os.path.join(applications_dir, version)
            if os.path.exists(target_dir):
                shutil.rmtree(target_dir)
                self.save_settings()
                self.update_version_selector()
                self.load_settings()
                self.update_select_button_state()
                self.check_start_game_button()
                logging.info("NWJS version %s uninstalled successfully.", version)
                QMessageBox.information(self, "Uninstall NWJS Version", f"NWJS version {version} uninstalled successfully.")
            else:
                logging.error("NWJS version %s not found.", version)
                QMessageBox.critical(self, "Error", f"NWJS version {version} not found.")

    def open_save_editor(self):
        save_editor_url = "https://saveeditor.online/"
        www_save_folder = os.path.join(self.last_selected_folder, "www", "save")
        save_folder = os.path.join(self.last_selected_folder, "save")

        try:
            subprocess.run(["open", save_editor_url], check=True)
            logging.info("Opened Save Editor website.")

            if os.path.exists(www_save_folder):
                subprocess.run(["open", www_save_folder], check=True)
                logging.info(f"Opened save folder: {www_save_folder}")
            elif os.path.exists(save_folder):
                subprocess.run(["open", save_folder], check=True)
                logging.info(f"Opened save folder: {save_folder}")
        except subprocess.CalledProcessError as e:
            logging.error(f"Failed to open: {str(e)}")
            QMessageBox.critical(self, "Error", f"Failed to open: {str(e)}")

    # Add or remove the cheat menu for RPG Maker MV and MZ games
    def add_cheat_menu(self, folder_path):
        www_folder_path = os.path.join(folder_path, "www")
        cheat_menu_js_path = os.path.join(os.path.dirname(__file__), 'Cheat_Menu.js')
        cheat_menu_css_path = os.path.join(os.path.dirname(__file__), 'Cheat_Menu.css')
        if os.path.exists(www_folder_path) and os.path.isdir(www_folder_path):
            logging.info("The 'www' folder exists in the selected folder, likely RPG MV.")
            if self.modify_MV_main_js(os.path.join(www_folder_path, 'js', 'main.js')):
                logging.info("RPG MV main.js has been patched with cheat menu.")
                self.copy_cheat_files(www_folder_path)
            else:
                logging.info("RPG MV main.js has already been patched with cheat menu.")
        else:
            logging.info("The 'www' folder does not exist in the selected folder, likely RPG MZ.")
            if self.modify_MZ_main_js(os.path.join(folder_path, 'js', 'main.js')):
                logging.info("RPG MZ main.js has been patched with cheat menu.")
                self.copy_cheat_files(folder_path)
            else:
                logging.info("RPG MZ main.js has already been patched with cheat menu.")

    def copy_cheat_files(self, target_folder):
        plugins_folder = os.path.join(target_folder, 'js', 'plugins')
        os.makedirs(plugins_folder, exist_ok=True)
        shutil.copy(os.path.join(os.path.dirname(__file__), 'Cheat_Menu.js'), plugins_folder)
        shutil.copy(os.path.join(os.path.dirname(__file__), 'Cheat_Menu.css'), plugins_folder)
        logging.info("Cheat_Menu.js and Cheat_Menu.css copied to %s", plugins_folder)
    
    def remove_cheat_menu(self, folder_path):
        www_folder_path = os.path.join(folder_path, "www")
        if os.path.exists(www_folder_path) and os.path.isdir(www_folder_path):
            logging.info("The 'www' folder exists in the selected folder, likely RPG MV.")
            if self.unmodify_MV_main_js(os.path.join(www_folder_path, 'js', 'main.js')):
                logging.info("RPG MV main.js has been unpatched from cheat menu.")
                self.remove_cheat_files(www_folder_path)
            else:
                logging.info("RPG MV main.js has not been patched with cheat menu.")
        else:
            logging.info("The 'www' folder does not exist in the selected folder, likely RPG MZ.")
            if self.unmodify_MZ_main_js(os.path.join(folder_path, 'js', 'main.js')):
                logging.info("RPG MZ main.js has been unpatched from cheat menu.")
                self.remove_cheat_files(folder_path)
            else:
                logging.info("RPG MZ main.js has not been patched with cheat menu.")
    
    def remove_cheat_files(self, target_folder):
        plugins_folder = os.path.join(target_folder, 'js', 'plugins')
        cheat_menu_js_path = os.path.join(plugins_folder, 'Cheat_Menu.js')
        cheat_menu_css_path = os.path.join(plugins_folder, 'Cheat_Menu.css')
        if os.path.exists(cheat_menu_js_path):
            os.remove(cheat_menu_js_path)
            logging.info("Removed Cheat_Menu.js from %s", plugins_folder)
        if os.path.exists(cheat_menu_css_path):
            os.remove(cheat_menu_css_path)
            logging.info("Removed Cheat_Menu.css from %s", plugins_folder)

    def modify_MV_main_js(self, file_path):
        try:
            with open(file_path, 'r') as file:
                lines = file.readlines()
        except Exception as e:
            logging.error("Failed to read main.js: %s", str(e))
            return False
        
        new_lines = [
            "PluginManager._path= 'js/plugins/';\n",
            "PluginManager.loadScript('Cheat_Menu.js');\n"
        ]
        
        if any(line in lines for line in new_lines):
            logging.info("Cheat menu lines already present in main.js")
            return False
        
        for i, line in enumerate(lines):
            if "PluginManager.setup($plugins);" in line:
                lines.insert(i + 1, new_lines[0])
                lines.insert(i + 2, new_lines[1])
                break
        
        try:
            with open(file_path, 'w') as file:
                file.writelines(lines)
            logging.info("main.js written successfully.")
        except Exception as e:
            logging.error("Failed to write main.js: %s", str(e))
            return False
        
        return True

    def modify_MZ_main_js(self, file_path):
        new_url = 'js/plugins/Cheat_Menu.js'
        try:
            with open(file_path, 'r') as file:
                content = file.read()
        except Exception as e:
            logging.error("Failed to read main.js: %s", str(e))
            return False

        script_urls_pattern = re.compile(r'const scriptUrls = \[(.*?)\];', re.DOTALL)
        match = script_urls_pattern.search(content)

        if match:
            script_urls_content = match.group(1)
            if new_url in script_urls_content:
                logging.info("Cheat menu URL already present in main.js")
                return False
            else:
                new_script_urls_content = f'    "{new_url}",\n' + script_urls_content
                new_content = content[:match.start(1)] + new_script_urls_content + content[match.end(1):]

                try:
                    with open(file_path, 'w') as file:
                        file.write(new_content)
                    logging.info("main.js written successfully.")
                except Exception as e:
                    logging.error("Failed to write main.js: %s", str(e))
                    return False
                return True
        else:
            logging.error('scriptUrls array not found in main.js.')
            return False
    
    def unmodify_MV_main_js(self, file_path):
        try:
            with open(file_path, 'r') as file:
                lines = file.readlines()
        except Exception as e:
            logging.error("Failed to read main.js: %s", str(e))
            return False

        lines_to_remove = [
            "PluginManager._path= 'js/plugins/';\n",
            "PluginManager.loadScript('Cheat_Menu.js');\n"
        ]

        if not any(line in lines for line in lines_to_remove):
            logging.info("Cheat menu lines not present in main.js")
            return False

        updated_lines = [line for line in lines if line not in lines_to_remove]

        try:
            with open(file_path, 'w') as file:
                file.writelines(updated_lines)
            logging.info("Cheat menu lines removed successfully.")
        except Exception as e:
            logging.error("Failed to write main.js: %s", str(e))
            return False

        return True
    
    def unmodify_MZ_main_js(self, file_path):
        new_url = 'js/plugins/Cheat_Menu.js'
        try:
            with open(file_path, 'r') as file:
                content = file.read()
        except Exception as e:
            logging.error("Failed to read main.js: %s", str(e))
            return False

        script_urls_pattern = re.compile(r'const scriptUrls = \[(.*?)\];', re.DOTALL)
        match = script_urls_pattern.search(content)

        if not match:
            logging.error('scriptUrls array not found in main.js.')
            return False

        script_urls_content = match.group(1)
        if new_url not in script_urls_content:
            logging.info("Cheat menu URL not present in main.js")
            return False

        new_script_urls_content = re.sub(
            r"\s*\"" + re.escape(new_url) + r"\",,?\n?", '',
            script_urls_content
        )
        new_content = content[:match.start(1)] + new_script_urls_content + content[match.end(1):]

        try:
            with open(file_path, 'w') as file:
                file.write(new_content)
            logging.info("Cheat menu URL removed successfully.")
        except Exception as e:
            logging.error("Failed to write main.js: %s", str(e))
            return False

        return True

    # Optimize space by removing unnecessary files and folders that are only used by windows
    # RPG MV and MZ games only
    def optimize_space(self):
        files_to_remove = [
            "credits.html", "d3dcompiler_47.dll", "ffmpeg.dll", "icudtl.dat", "libEGL.dll",
            "libGLESv2.dll", "node.dll", "nw.dll", "nw_100_percent.pak", "nw_200_percent.pak",
            "nw_elf.dll", "resources.pak", "v8_context_snapshot.bin", "vk_swiftshader.dll",
            "vk_swiftshader_icd.json", "vulkan-1.dll"
        ]
        
        folders_to_remove = ["locales", "swiftshader"]
        
        for root, dirs, files in os.walk(self.last_selected_folder):
            for file in files:
                file_path = os.path.join(root, file)
                if file in files_to_remove or file.endswith(".exe"):
                    try:
                        os.remove(file_path)
                        logging.info("Removed file: %s", file_path)
                    except Exception as e:
                        logging.error("Failed to remove file (already optimized?) %s: %s", file_path, str(e))
            
            for folder in dirs:
                folder_path = os.path.join(root, folder)
                if folder in folders_to_remove:
                    try:
                        shutil.rmtree(folder_path)
                        logging.info("Removed folder: %s", folder_path)
                    except Exception as e:
                        logging.error("Failed to remove folder (already optimized?) %s: %s", folder_path, str(e))

    def open_log(self):
        try:
            with open(LOG_FILE, 'r') as file:
                log_contents = file.read()
        except Exception as e:
            logging.error("Failed to read log file: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to read log file: {str(e)}")
            return

        dialog = QDialog(self)
        dialog.setWindowTitle("Log Contents")
        dialog.resize(600, 400)
        layout = QVBoxLayout(dialog)

        text_edit = QPlainTextEdit(dialog)
        text_edit.setPlainText(log_contents)
        text_edit.setReadOnly(True)
        layout.addWidget(text_edit)

        buttons = QDialogButtonBox(QDialogButtonBox.Ok, dialog)
        buttons.accepted.connect(dialog.accept)
        layout.addWidget(buttons)

        dialog.exec()

def main():
    check_appdir()

    # Set up logging after ensuring the directory exists
    logging.basicConfig(filename=LOG_FILE, level=logging.DEBUG, 
                        format='%(asctime)s - %(levelname)s - %(message)s')

    # Redirect stdout and stderr to log file
    sys.stdout = open(LOG_FILE, 'a')
    sys.stderr = open(LOG_FILE, 'a')
    
    app = QApplication(sys.argv)
    window = FolderPathApp()
    window.show()
    sys.exit(app.exec())

if __name__ == "__main__":
    main()
