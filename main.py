import sys
from runtime_arch import VERSION, ensure_supported_host, require_arm64, require_nwjs_arm64

# Source startup must explain unsupported hosts even when Qt cannot be imported.
if __name__ == "__main__":
    try:
        ensure_supported_host()
    except RuntimeError as exc:
        raise SystemExit(str(exc))

import os
import json
import subprocess
import requests
import zipfile
import shutil
import tempfile
import platform
import chardet
import logging
import re
from pathlib import Path
from functools import partial
from PySide6.QtWidgets import (QApplication, QMainWindow, QPushButton, QFileDialog,
                               QVBoxLayout, QHBoxLayout, QWidget, QMessageBox, QDialog, QComboBox,
                               QDialogButtonBox, QScrollArea, QGroupBox, QFormLayout, QLabel, QCheckBox, QProgressDialog, QLineEdit, QPlainTextEdit)
from PySide6.QtCore import QTimer, QDateTime, Qt
from nwjs_installer import HelperProcess, installed_versions, installation_lock, LOCK_NAME

current_version = "3.3.4"
config_version = ""
latest_commit_sha = ""
last_commit_sha = ""
latest_build_number = 0
last_build_number = 0

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

# EasyRPG advanced settings
default_enabled_easyrpg_settings = {
            "--autobattle-algo": False, "--enemyai-algo": False, "--config-path": False,
            "--encoding": False, "--engine": False, "--font1": False, "--font1-size": False,
            "--font2": False, "--font2-size": False, "--font-path": False, "--language": False,
            "--load-game-id": False, "--log-file": False, "--new-game": False, "--no-log-color": False,
            "--no-rtp": False, "--patch-antilag-switch": False, "--patch-common-this": False,
            "--patch-direct-menu": False, "--patch-dynrpg": False, "--patch-easyrpg": False,
            "--patch-key-patch": False, "--patch-maniac": False, "--patch-pic-unlock": False,
            "--patch-rpg2k3-cmds": False, "--no-patch": False, "--record-input": False,
            "--replay-input": False, "--rtp-path": False, "--save-path": False, "--seed": False,
            "--fps-limit": False, "--no-fps-limit": False, "--game-resolution": False,
            "--pause-focus-lost": False, "--no-pause-focus-lost": False, "--scaling": False,
            "--show-fps": False, "--fps-render-window": False, "--no-show-fps": False,
            "--stretch": False, "--no-stretch": False, "--vsync": False, "--no-vsync": False,
            "--no-audio": False, "--music-volume": False, "--sound-volume": False,
            "--soundfont": False, "--soundfont-path": False, "--battle-test": False,
            "--hide-title": False, "--start-map-id": False, "--start-party": False,
            "--start-position": False, "--test-play": False
        }

default_advanced_easyrpg_settings = {
            "--autobattle-algo": "RPG_RT", "--enemyai-algo": "RPG_RT", "--config-path": "",
            "--encoding": "", "--engine": "", "--font1": "", "--font1-size": 12,
            "--font2": "", "--font2-size": 12, "--font-path": "", "--language": "",
            "--load-game-id": "", "--log-file": "", "--new-game": False, "--no-log-color": False,
            "--no-rtp": False, "--patch-antilag-switch": "", "--patch-common-this": False,
            "--patch-direct-menu": "", "--patch-dynrpg": False, "--patch-easyrpg": False,
            "--patch-key-patch": False, "--patch-maniac": 1, "--patch-pic-unlock": False,
            "--patch-rpg2k3-cmds": False, "--no-patch": False, "--project-path": "",
            "--record-input": "", "--replay-input": "", "--rtp-path": "", "--save-path": "",
            "--seed": "", "--fps-limit": 60, "--no-fps-limit": False, "--game-resolution": "",
            "--pause-focus-lost": False, "--no-pause-focus-lost": False, "--scaling": "",
            "--show-fps": False, "--fps-render-window": False, "--no-show-fps": False,
            "--stretch": False, "--no-stretch": False, "--vsync": True, "--no-vsync": False,
            "--no-audio": False, "--music-volume": 100, "--sound-volume": 100, "--soundfont": "",
            "--soundfont-path": "", "--battle-test": "", "--hide-title": False, "--start-map-id": "",
            "--start-party": "", "--start-position": "", "--test-play": False
        }

# Set up logging
LOG_FILE = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/log.txt")

def check_appdir():
    ensure_supported_host()
    app_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher")
    if not os.path.exists(app_dir):
        os.makedirs(app_dir)
    # Clear log file on startup
    with open(LOG_FILE, 'w') as file:
        file.write("")

class RPGMLauncher(QMainWindow):
    SETTINGS_FILE = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/settings.json")

    def __init__(self):
        ensure_supported_host()
        super().__init__()

        self._nwjs_busy = False
        self._nwjs_closing = False
        self.nwjs_process = HelperProcess(parent=self)
        self.nwjs_process.done.connect(self._nwjs_done)
        self.nwjs_process.progress.connect(self._nwjs_progress)

        self.setWindowTitle("RPG Maker Launcher")
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

        self.optimize_space_checkbox = QCheckBox("Optimize Space (Unavailable: original files are protected)", self)
        self.optimize_space_checkbox.setEnabled(False)
        self.layout.addWidget(self.optimize_space_checkbox)

        self.disable_protection_checkbox = QCheckBox("Disable Protection (Not Recommended)", self)
        self.layout.addWidget(self.disable_protection_checkbox)

        self.getpixel_patch_checkbox = QCheckBox("Patch Bitmap.getPixel (Fix NaN/Infinity Crash)", self)
        self.getpixel_patch_checkbox.setChecked(True)
        self.layout.addWidget(self.getpixel_patch_checkbox)

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

        self.install_button = QPushButton("Install NWJS Version", self)
        self.layout.addWidget(self.install_button)

        self.uninstall_button = QPushButton("Uninstall Component", self)
        self.layout.addWidget(self.uninstall_button)

        self.mkxpz_advanced_button = QPushButton("MKXP-Z Advanced Settings", self)
        self.layout.addWidget(self.mkxpz_advanced_button)

        self.easyrpg_advanced_button = QPushButton("EasyRPG Advanced Settings", self)
        self.layout.addWidget(self.easyrpg_advanced_button)

        self.open_log_button = QPushButton("Open Log", self)
        self.layout.addWidget(self.open_log_button)

        self.install_button.clicked.connect(self.install_nwjs)
        self.uninstall_button.clicked.connect(self.uninstall_menu)
        self.select_button.clicked.connect(self.select_folder)
        self.start_game_button.clicked.connect(self.start_game)
        self.export_button.clicked.connect(self.export_standalone_app)
        self.mkxpz_advanced_button.clicked.connect(self.open_mkxpz_advanced_settings)
        self.easyrpg_advanced_button.clicked.connect(self.open_easyrpg_advanced_settings)
        self.open_log_button.clicked.connect(self.open_log)

        self.update_version_selector()
        self.load_settings()
        self.update_select_button_state()
        self.check_start_game_button()

    def show_instructions(self):
        instructions = (
            "Video Tutorial: https://www.youtube.com/watch?v=NIRiKIjhXHk\n"
            "_____________________________________________________________________________\n"
            "RPG Maker MV and MZ games:\n\n"
            "1. Click 'Install NWJS Version' to download and install the required version of NWJS.\n\n"
            "2. Click 'Select Game Folder' to choose the folder containing the RPG Maker game.\n\n"
            "3. Optional Options:\n"
            "   - 'Optimize Space': Unavailable to protect original game files.\n"
            "   - 'Disable Protection': Allows child_process to be used and allows outbound connections.\n"
            "   - 'Patch Bitmap.getPixel': Fixes crashes related to NaN/Infinity in Bitmap.getPixel.\n\n"
            "4. Click 'Start Game' to launch the game using the selected NWJS version.\n\n"
            "5. Click 'Export as Standalone App' to create a standalone application for the game.\n\n"
            "6. Click 'Uninstall Component' to remove installed versions of NWJS or other components.\n"
            "_____________________________________________________________________________\n"
            "RPG Maker 2000, 2003, XP, VX, and VX Ace games:\n\n"
            "1. Move game folder to anywhere but the 'Downloads' folder. (XP, VX, VX Ace only)\n\n"
            "2. Click 'Select Game Folder' to choose the folder containing the RPG Maker game.\n\n"
            "3. Click 'Start Game' to launch the game. You may be prompted to install MKXP-Z or EasyRPG."
        )
        
        dialog = QDialog(self)
        dialog.setWindowTitle("Instructions")
        dialog.resize(650, 590)
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
            "athanadoc05 - Donator\n\n"
            "mkxp-z maintainers - MKXP-Z\n\n"
            "SynthFont developers - Soundfont\n\n"
            "Orochimarufan - Kawariki Patches\n\n"
            "EasyRPG Team - EasyRPG Player\n\n"
            "xem/pgm - Video Tutorial"
        )
        
        dialog = QDialog(self)
        dialog.setWindowTitle("Credits")
        dialog.resize(650, 400)
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
                    self.optimize_space_checkbox.setChecked(False)
                    self.disable_protection_checkbox.setChecked(settings.get('disable_protection', False))
                    self.getpixel_patch_checkbox.setChecked(settings.get('patch_getpixel', True))
                    last_version = settings.get('last_selected_version')
                    if last_version and last_version in [self.version_selector.itemText(i) for i in range(self.version_selector.count())]:
                        self.version_selector.setCurrentText(last_version)
                    self.last_selected_folder = settings.get('last_selected_folder', os.path.expanduser("~/Downloads"))
                    global config_version
                    config_version = settings.get('launcher_version', "")
                    global last_commit_sha
                    last_commit_sha = settings.get('last_commit_sha', "")
                    global last_build_number
                    last_build_number = settings.get('last_build_number', 0)
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
            'optimize_space': self.optimize_space_checkbox.isChecked(),
            'disable_protection': self.disable_protection_checkbox.isChecked(),
            'patch_getpixel': self.getpixel_patch_checkbox.isChecked(),
            'last_selected_version': self.version_selector.currentText(),
            'last_selected_folder': self.last_selected_folder,
            'launcher_version': current_version,
            'last_commit_sha': last_commit_sha,
            'last_build_number': last_build_number
        }
        with open(self.SETTINGS_FILE, 'w') as file:
            json.dump(settings, file, indent=4)
    
    # Remove old configuration files every update
    def remove_configs(self):
        enabled_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/enabled-mkxpz-settings.json")
        advanced_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/mkxpz-advanced.json")
        easyrpg_enabled_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/enabled-easyrpg-settings.json")
        easyrpg_advanced_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/easyrpg-advanced.json")
        if os.path.exists(enabled_settings_file):
            os.remove(enabled_settings_file)
        if os.path.exists(advanced_settings_file):
            os.remove(advanced_settings_file)
        if os.path.exists(easyrpg_enabled_settings_file):
            os.remove(easyrpg_enabled_settings_file)
        if os.path.exists(easyrpg_advanced_settings_file):
            os.remove(easyrpg_advanced_settings_file)
        logging.info("Removed old MKXP-Z and EasyRPG configuration files.")

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
    
    def open_easyrpg_advanced_settings(self):
        dialog = QDialog(self)
        dialog.setWindowTitle("EasyRPG Advanced Settings")

        dialog.resize(500, 700)

        scroll_area = QScrollArea(dialog)
        scroll_area.setWidgetResizable(True)
        scroll_content = QWidget()
        scroll_layout = QVBoxLayout(scroll_content)
        scroll_area.setWidget(scroll_content)

        enabled_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/enabled-easyrpg-settings.json")
        advanced_settings_file = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/easyrpg-advanced.json")

        if not os.path.exists(enabled_settings_file):
            with open(enabled_settings_file, 'w') as file:
                json.dump(default_enabled_easyrpg_settings, file, indent=4)
            logging.info("Created enabled-easyrpg-settings.json with default values.")

        if not os.path.exists(advanced_settings_file):
            with open(advanced_settings_file, 'w') as file:
                json.dump(default_advanced_easyrpg_settings, file, indent=4)
            logging.info("Created easyrpg-advanced.json with default values.")

        with open(enabled_settings_file, 'r') as file:
            enabled_settings = json.load(file)
        
        with open(advanced_settings_file, 'r') as file:
            advanced_settings = json.load(file)

        optional_box = QGroupBox("Options", dialog)
        optional_layout = QVBoxLayout(optional_box)

        self.settings_widgets = {}
        for key, enabled in enabled_settings.items():
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

        scroll_layout.addWidget(optional_box)

        buttons_layout = QHBoxLayout()
        save_button = QPushButton("Save", dialog)
        reset_button = QPushButton("Reset", dialog)
        save_button.clicked.connect(lambda: self.save_easyrpg_advanced_settings(enabled_settings_file, advanced_settings_file))
        reset_button.clicked.connect(self.reset_easyrpg_advanced_settings)

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
    
    def save_easyrpg_advanced_settings(self, enabled_settings_file, advanced_settings_file):
        enabled_settings = {}
        advanced_settings = {}

        for key, controls in self.settings_widgets.items():
            widget = controls["widget"]
            enabled_checkbox = controls["enabled_checkbox"]

            if key in default_enabled_easyrpg_settings and default_enabled_easyrpg_settings[key] == "Force":
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
        QMessageBox.information(self, "Settings Saved", "EasyRPG settings have been saved successfully.")

    def reset_easyrpg_advanced_settings(self):
        for key, controls in self.settings_widgets.items():
            widget = controls["widget"]
            enabled_checkbox = controls["enabled_checkbox"]

            default_enabled = default_enabled_easyrpg_settings.get(key, False)
            default_value = default_advanced_easyrpg_settings.get(key)

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
        if self._nwjs_busy:
            event.ignore()
            self._nwjs_closing = True
            self._cancel_nwjs()
            return
        self.save_settings()
        super().closeEvent(event)

    def check_nwjs_installed(self):
        applications_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher")
        return bool(installed_versions(applications_dir))

    def _check_runtime(self, executable, warn=True):
        try:
            require_arm64(executable)
            return True
        except (OSError, ValueError) as exc:
            if warn:
                QMessageBox.critical(self, "Unsupported Runtime", str(exc))
            return False

    def _nwjs_executable(self):
        version = self.version_selector.currentText()
        directory = Path(os.path.expanduser("~/Library/Application Support/RPGM-Launcher")) / version
        try:
            if not VERSION.fullmatch(version):
                raise ValueError("Select a valid arm64 NW.js version (vX.Y.Z).")
            return str(require_nwjs_arm64(directory))
        except (OSError, ValueError) as exc:
            QMessageBox.critical(self, "Unsupported NW.js Runtime", str(exc))
            return None

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
                return self.check_mkxpz_installed(warn=False)
            else:
                logging.info("MKXPZ installation canceled by the user.")
                QMessageBox.information(self, "Installation Canceled", "MKXPZ installation was canceled.")
                return False
        return self._check_runtime(os.path.join(mkxpz_path, "Contents/MacOS/Z-universal"), warn)

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
    
    def check_easyrpg_installed(self, warn=True):
        easyrpg_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/EasyRPG Player.app")
        if not os.path.exists(easyrpg_path):
            if not warn:
                return False
            install_response = QMessageBox.question(
                self, "EasyRPG Not Found", 
                "EasyRPG is required but not found. Would you like to install it now?", 
                QMessageBox.Yes | QMessageBox.No
            )
            if install_response == QMessageBox.Yes:
                self.install_easyrpg()
                return self.check_easyrpg_installed(warn=False)
            else:
                logging.info("EasyRPG installation canceled by the user.")
                QMessageBox.information(self, "Installation Canceled", "EasyRPG installation was canceled.")
                return False
        return self._check_runtime(os.path.join(easyrpg_path, "Contents/MacOS/EasyRPG Player"), warn)
    
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
            global last_commit_sha
            return last_commit_sha
    
    def check_easyrpg_update(self):
        try:
            response = requests.get("https://ci.easyrpg.org/job/player-macos/api/json", timeout=1)
            response.raise_for_status()
            data = response.json()
            global latest_build_number
            latest_build_number = data['builds'][0]['number']
            logging.info("Latest EasyRPG build number: %d", latest_build_number)
            return latest_build_number
        except requests.RequestException as e:
            logging.error("Failed to fetch latest EasyRPG build number: %s", str(e))
            global last_build_number
            return last_build_number


    # NWJS version selector
    def update_version_selector(self):
        applications_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher")
        selected = self.version_selector.currentText()
        versions = installed_versions(applications_dir)
        self.version_selector.clear()
        self.version_selector.addItems(versions)
        if selected in versions:
            self.version_selector.setCurrentText(selected)
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

    def check_package_json(self, folder_path):
        # Selecting/detecting a game must not modify it or its NWJS profile.
        return os.path.isfile(os.path.join(folder_path, 'package.json'))

    def prepare_package_json(self, folder_path):
        """Repair launch metadata only when launching or preparing an exported copy."""
        file_path = Path(folder_path) / 'package.json'
        try:
            raw_data = file_path.read_bytes()
            encoding = chardet.detect(raw_data)['encoding'] or 'utf-8'
            data = json.loads(raw_data.decode(encoding))
            if not isinstance(data, dict):
                raise ValueError("package.json must contain an object")
            original = data.copy()
            name = data.get('name', '')
            if not isinstance(name, str):
                raise ValueError("package.json name must be a string")
            if not name.strip():
                data['name'] = 'tempname'

            bg_path = Path(folder_path) / 'bg.js'
            known_bg = (data.get('bg-script') == 'bg.js' and bg_path.is_file()
                        and not bg_path.is_symlink()
                        and bg_path.read_bytes() == (Path(__file__).parent / 'bg.js').read_bytes())
            if self.disable_protection_checkbox.isChecked():
                if known_bg:
                    data.pop('bg-script')
            elif 'bg-script' not in data or known_bg:
                self.add_protection_script(folder_path)
                data['bg-script'] = 'bg.js'
            else:
                QMessageBox.warning(self, "Protection not applied",
                                    "The game's existing bg-script was preserved. Launcher protection was not applied.")

            if data != original:
                with open(file_path, 'w', encoding=encoding) as file:
                    json.dump(data, file, indent=4)
            return True
        except (OSError, ValueError) as e:
            logging.error("Failed to prepare package.json: %s", e)
            QMessageBox.critical(self, "Error", f"Failed to prepare package.json: {e}")
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

    def add_protection_script(self, folder_path):
        scripts = ('bg.js', 'disable-child.js', 'disable-net.js')
        # Preflight every collision before creating anything; never replace user scripts.
        for name in scripts:
            target = Path(folder_path) / name
            source = Path(__file__).parent / name
            if target.is_symlink() or (target.exists() and target.read_bytes() != source.read_bytes()):
                raise ValueError(f"Refusing to overwrite existing protection script: {target}")
        for name in scripts:
            target = Path(folder_path) / name
            if not target.exists():
                with target.open('xb') as file:
                    file.write((Path(__file__).parent / name).read_bytes())
        logging.info("Protection scripts available in the game folder.")

    def patch_rpgm_getpixel(self, game_path, enable=True):
        # --- RPG Maker MV/MZ patch helpers ---
        RPGM_GETPIXEL_PATCH_BEGIN = "/* RPGM-Launcher getPixel finite patch begin */"
        RPGM_GETPIXEL_PATCH_END = "/* RPGM-Launcher getPixel finite patch end */"

        """Patch and unpatch Bitmap.prototype.getPixel in RPG Maker MV/MZ core JS.

        Looks for:
        - <game>/js or <game>/www/js
        - rmmz_core.js or rpg_core.js

        When enabled, inserts a finite-check guard right after the line:
        Bitmap.prototype.getPixel = function (x, y) {

        When disabled, removes the guard if it was previously inserted (with or without markers).
        """
        result = {"ok": False, "changed": False, "patched": False, "js_file": None, "message": ""}

        if not game_path or not os.path.isdir(game_path):
            result["message"] = "Invalid game path."
            return result

        # Find JS folder
        js_dir = None
        js_candidate_1 = os.path.join(game_path, "js")
        js_candidate_2 = os.path.join(game_path, "www", "js")
        if os.path.isdir(js_candidate_1):
            js_dir = js_candidate_1
        elif os.path.isdir(js_candidate_2):
            js_dir = js_candidate_2

        if not js_dir:
            result["message"] = "Could not find a 'js' or 'www/js' folder in the game directory."
            return result

        # Pick core file (prefer MZ)
        core_file = None
        mz_core = os.path.join(js_dir, "rmmz_core.js")
        mv_core = os.path.join(js_dir, "rpg_core.js")
        if os.path.isfile(mz_core):
            core_file = mz_core
        elif os.path.isfile(mv_core):
            core_file = mv_core

        if not core_file:
            result["message"] = "Could not find rmmz_core.js or rpg_core.js in the JS folder."
            return result

        result["js_file"] = core_file

        try:
            raw = Path(core_file).read_text(encoding="utf-8", errors="replace")
        except Exception as e:
            result["message"] = f"Failed to read core JS file: {e}"
            return result

        # Preserve newline style
        nl = "\r\n" if "\r\n" in raw else "\n"

        # Match the getPixel function signature line (spacing may differ across versions)
        sig_re = re.compile(
            r"^\s*Bitmap\.prototype\.getPixel\s*=\s*function\s*\(\s*x\s*,\s*y\s*\)\s*\{\s*$",
            re.MULTILINE,
        )
        m = sig_re.search(raw)
        if not m:
            result["message"] = "Could not locate 'Bitmap.prototype.getPixel = function (x, y) {' in the core JS file."
            return result

        # Compute insertion/removal position: immediately after the signature line's newline
        sig_line_end = raw.find(nl, m.end())
        if sig_line_end == -1:
            # Signature is last line
            sig_line_end = len(raw)
            after_sig = len(raw)
        else:
            after_sig = sig_line_end + len(nl)

        patch_block = (
            f"    {RPGM_GETPIXEL_PATCH_BEGIN}{nl}"
            f"    if (!Number.isFinite(x) || !Number.isFinite(y)) {{{nl}"
            f"        return '#000000';{nl}"
            f"    }}{nl}"
            f"    {RPGM_GETPIXEL_PATCH_END}{nl}"
        )

        # Fast path: already patched (marker-based)
        if RPGM_GETPIXEL_PATCH_BEGIN in raw and RPGM_GETPIXEL_PATCH_END in raw:
            if enable:
                result.update({"ok": True, "changed": False, "patched": True, "message": "Already patched."})
                return result
            # Disable -> remove marker block
            begin_idx = raw.find(RPGM_GETPIXEL_PATCH_BEGIN)
            end_idx = raw.find(RPGM_GETPIXEL_PATCH_END, begin_idx)
            if end_idx != -1:
                end_idx = raw.find(nl, end_idx)
                if end_idx == -1:
                    end_idx = len(raw)
                else:
                    end_idx += len(nl)

                prefix = raw[:begin_idx]
                if not prefix.endswith(nl):
                    prefix = prefix.rstrip(" \t") + nl
                new_raw = prefix + raw[end_idx:]
                if new_raw != raw:
                    try:
                        Path(core_file).write_text(new_raw, encoding="utf-8")
                        result.update({"ok": True, "changed": True, "patched": False, "message": "Patch removed."})
                    except Exception as e:
                        result["message"] = f"Failed to write core JS file: {e}"
                    return result

            result.update({"ok": True, "changed": False, "patched": False, "message": "Patch markers found but could not remove cleanly."})
            return result

        # Marker-less patch detection/removal right after the signature line
        # (Only inspect a small window after the signature to avoid deleting unrelated code.)
        window = raw[after_sig: after_sig + 600]

        snippet_re = re.compile(
            r"^\s*if\s*\(\s*!Number\.isFinite\(x\)\s*\|\|\s*!Number\.isFinite\(y\)\s*\)\s*\{\s*\r?\n"
            r"\s*return\s*['\"]#000000['\"];\s*\r?\n"
            r"\s*\}\s*\r?\n",
            re.MULTILINE,
        )
        snippet_m = snippet_re.search(window)

        if enable:
            if snippet_m:
                result.update({"ok": True, "changed": False, "patched": True, "message": "Already patched (no markers)."})
                return result

            # Insert patch block
            new_raw = raw[:after_sig] + patch_block + raw[after_sig:]
            try:
                Path(core_file).write_text(new_raw, encoding="utf-8")
                result.update({"ok": True, "changed": True, "patched": True, "message": "Patch applied."})
            except Exception as e:
                result["message"] = f"Failed to write core JS file: {e}"
            return result

        # disable
        if snippet_m:
            # Remove just the snippet region found in the window
            start = after_sig + snippet_m.start()
            end = after_sig + snippet_m.end()
            new_raw = raw[:start] + raw[end:]
            try:
                Path(core_file).write_text(new_raw, encoding="utf-8")
                result.update({"ok": True, "changed": True, "patched": False, "message": "Patch removed (no markers)."})
            except Exception as e:
                result["message"] = f"Failed to write core JS file: {e}"
            return result

        result.update({"ok": True, "changed": False, "patched": False, "message": "No patch found; nothing to remove."})
        return result

    def start_game(self):
        if self._nwjs_busy:
            return
        folder_path = self.last_selected_folder
        if not folder_path:
            logging.error("No folder selected.")
            QMessageBox.critical(self, "Error", "No folder selected.")
            return
        
        if self.check_package_json(folder_path):
            logging.info("Launching game using NWJS.")
            if not self._nwjs_executable():
                return

            if not self.prepare_package_json(folder_path):
                return

            patch_res = self.patch_rpgm_getpixel(folder_path, enable=self.getpixel_patch_checkbox.isChecked())
            if not patch_res.get("ok"):
                logging.warning("getPixel patch: %s", patch_res.get("message"))
                if self.getpixel_patch_checkbox.isChecked():
                    QMessageBox.warning(self, "getPixel patch", f"Could not apply getPixel patch:\n{patch_res.get('message')}, Not an RPG maker game?")
            else:
                logging.info("getPixel patch: %s", patch_res.get("message"))

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
        
        nwjs_path = self._nwjs_executable()
        if not nwjs_path:
            return
        subprocess.Popen([nwjs_path, folder_path], stdout=open(LOG_FILE, 'a'), stderr=subprocess.STDOUT)
        logging.info("Game launched using NWJS version %s.", selected_version)
        logging.info("NWJS log:")

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
            logging.info("Full MKXP-Z configuration: %s", mkxp_config)
        except Exception as e:
            logging.error("Failed to update mkxp.json: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to update mkxp.json: {str(e)}")
            return

        mkxpz_executable_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/Z-universal.app/Contents/MacOS/Z-universal")
        try:
            subprocess.Popen(["/usr/bin/arch", "-arm64", mkxpz_executable_path, folder_path], stdout=open(LOG_FILE, 'a'), stderr=subprocess.STDOUT)
            logging.info("MKXPZ game launched from folder: %s", folder_path)
            logging.info("MKXPZ log:")
        except Exception as e:
            logging.error("Failed to launch MKXPZ: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to launch MKXPZ: {str(e)}")
    
    def launch_easyrpg_game(self, folder_path):
        if self.check_easyrpg_installed(warn=False):
            global last_build_number
            logging.info("Checking for EasyRPG updates.")
            logging.info("Last build number: %d", last_build_number)
            if last_build_number != self.check_easyrpg_update():
                update_response = QMessageBox.question(self, "EasyRPG Update Available",
                                                       "A new update is available for EasyRPG Player. Would you like to update now?",
                                                       QMessageBox.Yes | QMessageBox.No)
                if update_response == QMessageBox.Yes:
                    self.install_easyrpg()

        easyrpg_executable_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/EasyRPG Player.app/Contents/MacOS/EasyRPG Player")
        if not self.check_easyrpg_installed():
            logging.error("EasyRPG Player not found at %s", easyrpg_executable_path)
            QMessageBox.critical(self, "Error", "EasyRPG Player not found. Please install it first.")
            return
        
        extra_options = []

        if os.path.isfile(os.path.expanduser("~/Library/Application Support/RPGM-Launcher/enabled-easyrpg-settings.json")) and os.path.isfile(os.path.expanduser("~/Library/Application Support/RPGM-Launcher/easyrpg-advanced.json")):
            with open(os.path.expanduser("~/Library/Application Support/RPGM-Launcher/enabled-easyrpg-settings.json"), 'r') as file:
                enabled_settings = json.load(file)

            with open(os.path.expanduser("~/Library/Application Support/RPGM-Launcher/easyrpg-advanced.json"), 'r') as file:
                advanced_settings = json.load(file)

            for key, enabled in enabled_settings.items():
                if enabled in ["True", "Force"]:
                    if advanced_settings[key] == True:
                        extra_options.append(f"{key}")
                    elif advanced_settings[key]:
                        extra_options.append(f"{key}")
                        extra_options.append(advanced_settings[key])
        
        try:
            subprocess.Popen(["/usr/bin/arch", "-arm64", easyrpg_executable_path, "--window", "--project-path", folder_path] + extra_options, stdout=open(LOG_FILE, 'a'), stderr=subprocess.STDOUT)
            logging.info("EasyRPG game launched from folder: %s (arm64)", folder_path)
            logging.info("Appended options: %s", extra_options)
            logging.info("EasyRPG log:")
        except Exception as e:
            logging.error("Failed to launch EasyRPG: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to launch EasyRPG: {str(e)}")

    def export_standalone_app(self):
        if self._nwjs_busy:
            return
        app_name, ok = self.get_app_name()
        if not ok:
            return

        destination_folder = QFileDialog.getExistingDirectory(self, "Select Destination Folder")
        if not destination_folder:
            return

        folder_path = self.last_selected_folder

        # Helper to detect paths within the destination .app
        def _is_within(child_path: str, parent_path: str) -> bool:
            child_abs = os.path.abspath(child_path)
            parent_abs = os.path.abspath(parent_path)
            # Ensure trailing separator for proper prefix check
            return child_abs == parent_abs or child_abs.startswith(parent_abs + os.sep)

        try:
            if self.check_package_json(folder_path):
                # Validate before copying or repairing any game files.
                if not self._nwjs_executable():
                    return
                selected_version = self.version_selector.currentText()
                if not selected_version:
                    logging.error("No NWJS version selected.")
                    QMessageBox.critical(self, "Error", "No NWJS version selected.")
                    return

                nwjs_dir = os.path.expanduser(f"~/Library/Application Support/RPGM-Launcher/{selected_version}")
                nwjs_app_src = os.path.join(nwjs_dir, "nwjs.app")
                nwjs_app_dst = os.path.join(destination_folder, app_name + ".app")

                # Count files while skipping anything inside the destination .app
                total_files = 0
                for root, dirs, files in os.walk(self.last_selected_folder, topdown=True):
                    # prune traversal into the destination app bundle (avoids infinite loop)
                    dirs[:] = [d for d in dirs if not _is_within(os.path.join(root, d), nwjs_app_dst)]
                    # if somehow root itself is inside the app bundle, skip it
                    if _is_within(root, nwjs_app_dst):
                        continue
                    total_files += len(files)

                progress_dialog = QProgressDialog("Exporting standalone app...", "Cancel", 0, total_files, self)
                progress_dialog.setWindowModality(Qt.WindowModal)
                progress_dialog.setMinimumDuration(0)
                progress_dialog.show()

                shutil.copytree(nwjs_app_src, nwjs_app_dst)

                app_nw_dir = os.path.join(nwjs_app_dst, "Contents", "Resources", "app.nw")
                os.makedirs(app_nw_dir, exist_ok=True)

                current_file_count = 0
                for root, dirs, files in os.walk(self.last_selected_folder, topdown=True):
                    # prune traversal into the destination app bundle (avoids infinite loop)
                    dirs[:] = [d for d in dirs if not _is_within(os.path.join(root, d), nwjs_app_dst)]
                    if _is_within(root, nwjs_app_dst):
                        continue

                    for file in files:
                        s = os.path.join(root, file)
                        # also skip files that would be inside the destination app, just in case
                        if _is_within(s, nwjs_app_dst):
                            continue

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
                if not self.prepare_package_json(app_nw_dir):
                    return
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

                total_files = 0
                for root, dirs, files in os.walk(folder_path, topdown=True):
                    # prune traversal into the destination app bundle
                    dirs[:] = [d for d in dirs if not _is_within(os.path.join(root, d), mkxpz_app_dst)]
                    if _is_within(root, mkxpz_app_dst):
                        continue
                    # also ensure we don't count files inside the destination .app
                    total_files += sum(1 for f in files if not _is_within(os.path.join(root, f), mkxpz_app_dst))

                progress_dialog = QProgressDialog("Exporting standalone app...", "Cancel", 0, total_files, self)
                progress_dialog.setWindowModality(Qt.WindowModal)
                progress_dialog.setMinimumDuration(0)
                progress_dialog.show()

                current_file_count = 0
                for root, dirs, files in os.walk(folder_path, topdown=True):
                    # prune traversal into the destination app bundle
                    dirs[:] = [d for d in dirs if not _is_within(os.path.join(root, d), mkxpz_app_dst)]
                    if _is_within(root, mkxpz_app_dst):
                        continue

                    for file in files:
                        s = os.path.join(root, file)
                        if _is_within(s, mkxpz_app_dst):
                            continue  # extra guard; skip anything inside the destination .app

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
                        mkxp_config["preloadScript"] = [f"./{os.path.join('kawariki', 'preload.rb')}"]

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
                if not self.check_easyrpg_installed():
                    return
                easyrpg_app_src = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/EasyRPG Player.app")
                easyrpg_app_dst = os.path.join(destination_folder, app_name + ".app")

                os.mkdir(easyrpg_app_dst)

                game_dir = os.path.join(easyrpg_app_dst, "game")

                total_files = 0
                for root, dirs, files in os.walk(folder_path, topdown=True):
                    # prune traversal into the destination app bundle
                    dirs[:] = [d for d in dirs if not _is_within(os.path.join(root, d), easyrpg_app_dst)]
                    if _is_within(root, easyrpg_app_dst):
                        continue
                    # don't count files inside the destination .app
                    total_files += sum(1 for f in files if not _is_within(os.path.join(root, f), easyrpg_app_dst))

                progress_dialog = QProgressDialog("Exporting standalone app...", "Cancel", 0, total_files, self)
                progress_dialog.setWindowModality(Qt.WindowModal)
                progress_dialog.setMinimumDuration(0)
                progress_dialog.show()

                current_file_count = 0
                for root, dirs, files in os.walk(folder_path, topdown=True):
                    # prune traversal into the destination app bundle
                    dirs[:] = [d for d in dirs if not _is_within(os.path.join(root, d), easyrpg_app_dst)]
                    if _is_within(root, easyrpg_app_dst):
                        continue

                    for file in files:
                        s = os.path.join(root, file)
                        if _is_within(s, easyrpg_app_dst):
                            continue  # extra guard

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

                logging.info("Applying advanced settings...")

                extra_options = ""

                if os.path.isfile(os.path.expanduser("~/Library/Application Support/RPGM-Launcher/enabled-easyrpg-settings.json")) and os.path.isfile(os.path.expanduser("~/Library/Application Support/RPGM-Launcher/easyrpg-advanced.json")):
                    with open(os.path.expanduser("~/Library/Application Support/RPGM-Launcher/enabled-easyrpg-settings.json"), 'r') as file:
                        enabled_settings = json.load(file)

                    with open(os.path.expanduser("~/Library/Application Support/RPGM-Launcher/easyrpg-advanced.json"), 'r') as file:
                        advanced_settings = json.load(file)

                    for key, enabled in enabled_settings.items():
                        if enabled in ["True", "Force"]:
                            if advanced_settings[key] == True:
                                extra_options += f" {key}"
                            elif advanced_settings[key]:
                                extra_options += f" {key} \"{advanced_settings[key]}\""
                
                with open(os.path.join(easyrpg_app_dst, app_name),'a') as f:
                    f.write(extra_options)

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

    # Only NW.js acquisition moves to Go; the rest of the launcher stays Python.
    def install_nwjs(self):
        if self._nwjs_busy:
            return
        self._nwjs_busy = True
        self.central_widget.setEnabled(False)
        self._nwjs_phase = 'versions'
        self._nwjs_dialog = QProgressDialog("Fetching NW.js versions...", "Cancel", 0, 0, self)
        self._nwjs_dialog.setAutoClose(False)
        self._nwjs_dialog.setAutoReset(False)
        self._nwjs_dialog.setMinimumDuration(0)
        self._nwjs_dialog.canceled.connect(self._cancel_nwjs)
        self._nwjs_dialog.show()
        self.nwjs_process.start(['versions'], 'versions')

    def _cancel_nwjs(self):
        if not self._nwjs_busy:
            return
        if self.nwjs_process.running:
            self._nwjs_dialog.setLabelText("Cancelling NW.js helper; waiting for cleanup...")
            self.nwjs_process.cancel()
        else:
            self._finish_nwjs()

    def _nwjs_progress(self, progress):
        if not self._nwjs_busy:
            return
        completed, total = progress['completed'], progress['total']
        self._nwjs_dialog.setLabelText(
            f"NW.js {progress['phase']}: {completed / (1024 * 1024):.1f} MiB"
            + (f" / {total / (1024 * 1024):.1f} MiB" if total else ""))
        # Byte counts can exceed Qt's signed 32-bit progress range.
        self._nwjs_dialog.setRange(0, 1000 if total else 0)
        self._nwjs_dialog.setValue(min(1000, completed * 1000 // total) if total else 0)

    def _nwjs_done(self, result, error, cancelled):
        if cancelled or error:
            self._finish_nwjs(error)
            return
        if self._nwjs_phase == 'versions':
            self._nwjs_dialog.hide()
            versions = {v['version']: v['arches'] for v in result['versions']
                        if v['arches'] == ['arm64']}
            if not versions:
                self._finish_nwjs("No compatible NW.js releases were returned.")
                return
            version, ok = self.show_version_selection_dialog(versions.keys())
            if not self._nwjs_busy:
                return
            if not ok or version not in versions:
                self._finish_nwjs()
                return
            self._nwjs_version = version
            self._nwjs_root = os.path.expanduser("~/Library/Application Support/RPGM-Launcher")
            arguments = ['install', '--root', self._nwjs_root, '--version', version]
            self._nwjs_phase = 'install'
            self._nwjs_dialog.setLabelText("Installing NW.js...")
            self._nwjs_dialog.show()
            self.nwjs_process.start(arguments, 'complete')
            return
        expected = Path(self._nwjs_root).resolve() / self._nwjs_version / 'nwjs.app'
        if (result['version'] != self._nwjs_version or Path(result['path']) != expected
                or self._nwjs_version not in installed_versions(self._nwjs_root)):
            self._finish_nwjs("NW.js helper returned an unexpected or incomplete installation.")
            return
        try:
            self.update_version_selector()
            self.version_selector.setCurrentText(self._nwjs_version)
            self.save_settings()
        except OSError as exc:
            self._finish_nwjs(f"NW.js installed, but launcher settings could not be saved: {exc}")
            return
        self._finish_nwjs()
        QMessageBox.information(self, "Install NWJS Version", "NWJS installation completed successfully.")

    def _finish_nwjs(self, error=""):
        self._nwjs_dialog.hide()
        self._nwjs_dialog.deleteLater()
        self._nwjs_busy = False
        self.central_widget.setEnabled(True)
        self.update_select_button_state()
        self.check_start_game_button()
        if error:
            logging.error("NW.js helper: %s", error)
            if not self._nwjs_closing:
                QMessageBox.critical(self, "NW.js Installation", error)
        if self._nwjs_closing:
            QTimer.singleShot(0, self.close)

    def _publish_runtime(self, staged_app, target, executable_name):
        staged_app, target = Path(staged_app), Path(target)
        binary = staged_app / "Contents/MacOS" / executable_name
        if binary.is_symlink() or not binary.is_file():
            raise ValueError(f"Missing or unsafe runtime executable: {binary}")
        binary.chmod(binary.stat().st_mode | 0o100)
        require_arm64(binary)
        backup = target.with_name(f".{target.name}.previous")
        if (os.path.lexists(backup) or target.is_symlink()
                or (target.exists() and not target.is_dir())):
            raise ValueError(f"Refusing to replace an unsafe runtime or existing recovery copy: {backup}")
        had_previous = target.exists()
        if had_previous:
            os.rename(target, backup)
        try:
            os.rename(staged_app, target)
        except OSError:
            if had_previous:
                # Recovery stays outside staging, even if restoring it fails.
                os.rename(backup, target)
            raise
        if had_previous:
            shutil.rmtree(backup)

    # Install MKXP-Z, updates will be checked automatically
    def install_mkxpz(self):
        # The URL for the MKXP-Z universal build is under my fork because it needs to be built manually
        URL = "https://github.com/m5kro/mkxp-z/releases/download/launcher/Z-universal.zip"
        target_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/")
        stage = None
        try:
            stage = tempfile.TemporaryDirectory(prefix=".runtime-stage-", dir=target_dir)
            zip_file_path = os.path.join(stage.name, "download.zip")
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
                zip_ref.extractall(stage.name)
            self._publish_runtime(Path(stage.name) / "Z-universal.app",
                                  Path(target_dir) / "Z-universal.app", "Z-universal")

            progress_dialog.close()
            global last_commit_sha
            global latest_commit_sha
            last_commit_sha = latest_commit_sha
            QMessageBox.information(self, "MKXPZ Installation", "MKXPZ installed successfully.")
            

        except Exception as e:
            logging.error("Error installing MKXPZ: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to install MKXPZ: {str(e)}")
        finally:
            if stage is not None:
                stage.cleanup()
    
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
        target_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/")
        stage = None
        try:
            stage = tempfile.TemporaryDirectory(prefix=".runtime-stage-", dir=target_dir)
            zip_file_path = os.path.join(stage.name, "download.zip")
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
                zip_ref.extractall(stage.name)
            self._publish_runtime(Path(stage.name) / "EasyRPG Player.app",
                                  Path(target_dir) / "EasyRPG Player.app", "EasyRPG Player")

            progress_dialog.close()
            global last_build_number
            global latest_build_number
            last_build_number = latest_build_number
            QMessageBox.information(self, "EasyRPG Player Installation", "EasyRPG Player installed successfully.")
            
        except Exception as e:
            logging.error("Error installing EasyRPG Player: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to install EasyRPG Player: {str(e)}")
        finally:
            if stage is not None:
                stage.cleanup()

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

    def uninstall_menu(self):
        if self._nwjs_busy:
            return
        logging.info("Uninstall menu opened.")
        dialog = QDialog(self)
        dialog.setWindowTitle("Uninstall Component")
        layout = QVBoxLayout(dialog)
        label = QLabel("Select the component to uninstall:", dialog)
        layout.addWidget(label)
        combo_box = QComboBox(dialog)
        combo_box.addItems(["NWJS", "MKXP-Z", "EasyRPG Player", "ALL"])
        layout.addWidget(combo_box)
        buttons = QDialogButtonBox(QDialogButtonBox.Ok | QDialogButtonBox.Cancel, dialog)
        buttons.accepted.connect(dialog.accept)
        buttons.rejected.connect(dialog.reject)
        layout.addWidget(buttons)
        result = dialog.exec()
        if result == QDialog.Accepted:
            selected_option = combo_box.currentText()
            if selected_option == "NWJS":
                self.uninstall_nwjs()
            elif selected_option == "MKXP-Z":
                self.uninstall_mkxpz()
            elif selected_option == "EasyRPG Player":
                self.uninstall_easyrpg()
            elif selected_option == "ALL":
                self.uninstall_all()

    def uninstall_nwjs(self):
        if self._nwjs_busy:
            return
        applications_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher")
        versions = installed_versions(applications_dir)
        version, ok = self.show_version_selection_dialog(versions)
        if ok and version in versions:
            try:
                with installation_lock(applications_dir):
                    shutil.rmtree(os.path.join(applications_dir, version))
            except OSError as exc:
                QMessageBox.critical(self, "Uninstall NWJS Version", f"Cannot uninstall (another installer may be running): {exc}")
                return
            self.update_version_selector()
            self.save_settings()
            self.check_start_game_button()
            QMessageBox.information(self, "Uninstall NWJS Version", f"NWJS version {version} uninstalled successfully.")
    
    def uninstall_mkxpz(self):
        logging.info("Uninstalling MKXP-Z and associated files...")
        mkxpz_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/Z-universal.app")
        RTP_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/RTP")
        soundfont_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/GMGSx.SF2")
        kawariki_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/kawariki")

        if os.path.exists(mkxpz_path):
            shutil.rmtree(mkxpz_path)
            logging.info("MKXP-Z uninstalled successfully.")
        else:
            logging.error("MKXP-Z not found.")
            QMessageBox.critical(self, "Error", "MKXP-Z not found.")
        
        if os.path.exists(RTP_path):
            shutil.rmtree(RTP_path)
            logging.info("RTP uninstalled successfully.")
        else:
            logging.error("RTP not found.")
            QMessageBox.critical(self, "Error", "RTP not found.")
        
        if os.path.exists(soundfont_path):
            os.remove(soundfont_path)
            logging.info("Soundfont GMGSx.SF2 uninstalled successfully.")
        else:
            logging.error("Soundfont GMGSx.SF2 not found.")
            QMessageBox.critical(self, "Error", "Soundfont GMGSx.SF2 not found.")
        
        if os.path.exists(kawariki_path):
            shutil.rmtree(kawariki_path)
            logging.info("Kawariki patches uninstalled successfully.")
        else:
            logging.error("Kawariki patches not found.")
            QMessageBox.critical(self, "Error", "Kawariki patches not found.")
        
        QMessageBox.information(self, "Uninstall MKXP-Z", "MKXP-Z and associated files uninstalled successfully.")
    
    def uninstall_easyrpg(self, popup=True):
        logging.info("Uninstalling EasyRPG Player...")
        easyrpg_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/EasyRPG Player.app")

        if os.path.exists(easyrpg_path):
            shutil.rmtree(easyrpg_path)
            logging.info("EasyRPG Player uninstalled successfully.")
            if popup:
                QMessageBox.information(self, "Uninstall EasyRPG Player", "EasyRPG Player uninstalled successfully.")
        else:
            logging.error("EasyRPG Player not found.")
            QMessageBox.critical(self, "Error", "EasyRPG Player not found.")
    
    def uninstall_all(self):
        if self._nwjs_busy:
            return
        if not QMessageBox.question(self, "Uninstall All", "Are you sure you want to uninstall all components?", QMessageBox.Yes | QMessageBox.No) == QMessageBox.Yes:
            return
        applications_dir = Path(os.path.expanduser("~/Library/Application Support/RPGM-Launcher"))
        try:
            with installation_lock(applications_dir):
                # Keep root and lock inode: removing either would bypass other helpers' flock.
                for path in applications_dir.iterdir():
                    if path.name == LOCK_NAME:
                        continue
                    if path.is_dir() and not path.is_symlink():
                        shutil.rmtree(path)
                    else:
                        path.unlink()
        except OSError as exc:
            QMessageBox.critical(self, "Uninstall All", f"Cannot uninstall (another installer may be running): {exc}")
            return
        self.update_version_selector()
        self.check_start_game_button()
        QMessageBox.information(self, "Uninstall All", "All components uninstalled successfully.")

    def optimize_space(self):
        logging.warning("Optimize Space is unavailable: original game files must be preserved.")

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
    ensure_supported_host()
    check_appdir()

    # Set up logging after ensuring the directory exists
    logging.basicConfig(filename=LOG_FILE, level=logging.DEBUG, 
                        format='%(asctime)s - %(levelname)s - %(message)s')

    # Redirect stdout and stderr to log file
    sys.stdout = open(LOG_FILE, 'a')
    sys.stderr = open(LOG_FILE, 'a')

    logging.info("Launcher version: %s", current_version)
    logging.info("MacOS version: %s", platform.mac_ver()[0])
    logging.info("MacOS architecture: %s", platform.machine())
    
    app = QApplication(sys.argv)
    window = RPGMLauncher()
    window.show()
    sys.exit(app.exec())

if __name__ == "__main__":
    main()
