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
from evbunpack.__main__ import unpack_files
from PySide6.QtWidgets import (QApplication, QMainWindow, QPushButton, QFileDialog,
                               QVBoxLayout, QHBoxLayout, QWidget, QMessageBox, QDialog, QComboBox,
                               QDialogButtonBox, QFormLayout, QLabel, QCheckBox, QProgressDialog, QLineEdit, QPlainTextEdit)
from PySide6.QtCore import QTimer, QDateTime, Qt

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

        self.open_log_button = QPushButton("Open Log", self)
        self.layout.addWidget(self.open_log_button)

        self.install_button.clicked.connect(self.install_nwjs)
        self.uninstall_button.clicked.connect(self.uninstall_nwjs)
        self.select_button.clicked.connect(self.select_folder)
        self.start_game_button.clicked.connect(self.start_game)
        self.export_button.clicked.connect(self.export_standalone_app)
        self.open_save_editor_button.clicked.connect(self.open_save_editor)
        self.open_log_button.clicked.connect(self.open_log)

        self.update_version_selector()
        self.load_settings()
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
            "RPG Maker XP, VX, and VX Ace games:\n\n"
            "1. Move game folder to anywhere but the 'Downloads' folder.\n\n"
            "2. Click 'Select Game Folder' to choose the folder containing the RPG Maker game.\n\n"
            "3. Click 'Start Game' to launch the game using MKXP-Z. You may be prompted to install MKXP-Z."
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

    def show_credits(self):
        instructions = (
            "Special Thanks To:\n\n"
            "lecrolonk - Donator\n\n"
            "mkxp-z maintainers - MKXP-Z\n\n"
            "Andmi Kuzgri - Save Editor Online\n\n"
            "emerladCoder - Cheat Menu Plugin"
        )
        
        dialog = QDialog(self)
        dialog.setWindowTitle("Instructions")
        dialog.resize(650, 250)
        layout = QVBoxLayout(dialog)

        text_edit = QPlainTextEdit(dialog)
        text_edit.setPlainText(instructions)
        text_edit.setReadOnly(True)
        layout.addWidget(text_edit)

        buttons = QDialogButtonBox(QDialogButtonBox.Ok, dialog)
        buttons.accepted.connect(dialog.accept)
        layout.addWidget(buttons)

        dialog.exec()

    def load_settings(self):
        if os.path.exists(self.SETTINGS_FILE):
            with open(self.SETTINGS_FILE, 'r') as file:
                settings = json.load(file)
                self.extract_checkbox.setChecked(settings.get('extract_game_en', False))
                self.cheat_menu_checkbox.setChecked(settings.get('add_cheat_menu', False))
                self.optimize_space_checkbox.setChecked(settings.get('optimize_space', False))
                last_version = settings.get('last_selected_version')
                if last_version and last_version in [self.version_selector.itemText(i) for i in range(self.version_selector.count())]:
                    self.version_selector.setCurrentText(last_version)
                self.last_selected_folder = settings.get('last_selected_folder', os.path.expanduser("~/Downloads"))
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
            'last_selected_folder': self.last_selected_folder
        }
        with open(self.SETTINGS_FILE, 'w') as file:
            json.dump(settings, file, indent=4)

    def closeEvent(self, event):
        self.save_settings()
        super().closeEvent(event)

    def check_nwjs_installed(self):
        applications_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher")
        return os.path.exists(applications_dir) and any(os.path.isdir(os.path.join(applications_dir, v)) for v in os.listdir(applications_dir))

    def check_mkxpz_installed(self):
        mkxpz_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/Z-universal.app")
        if not os.path.exists(mkxpz_path):
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

    def update_version_selector(self):
        applications_dir = os.path.expanduser("~/Library/Application Support/RPGM-Launcher")
        if os.path.exists(applications_dir):
            # Only include directories that start with 'v'
            versions = [
                v for v in os.listdir(applications_dir) 
                if os.path.isdir(os.path.join(applications_dir, v)) and v.startswith("v")
            ]
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
            
            if not self.check_package_json(folder_path):
                logging.error("No package.json found in the selected folder.")
                if self.check_game_ini(folder_path):
                    logging.info("Game.ini file found in the folder.")
                else:
                    logging.error("No Game.ini file found in the selected folder.")
                    QMessageBox.critical(self, "Error", "No package.json or Game.ini file found in the selected folder.")
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

    def check_game_ini(self, folder_path):
        game_ini_path = os.path.join(folder_path, "Game.ini")
        return os.path.exists(game_ini_path)
    
    def get_rtp_value(self, folder_path):
        game_ini_path = os.path.join(folder_path, "Game.ini")
        with open(game_ini_path, 'r') as file:
            for line in file:
                match = re.match(r"RTP=(.*)", line)
                if match:
                    return match.group(1).strip()  # Returns the value after '='
        logging.warning("RTP value not found in Game.ini. Assuming Standard RTP (RPG XP).")
        return "Standard"

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

        if self.check_package_json(folder_path):
            logging.info("Launching game using NWJS.")

            if self.extract_checkbox.isChecked():
                self.check_and_unpack_game_en(folder_path)

            if self.optimize_space_checkbox.isChecked():
                self.optimize_space()

            if self.cheat_menu_checkbox.isChecked():
                self.add_cheat_menu(folder_path)
            
            self.launch_nwjs_game(folder_path)
        elif self.check_game_ini(folder_path):
            logging.info("Launching game using MKXPZ.")
            self.launch_mkxpz_game(folder_path)
        else:
            logging.error("Neither package.json nor .ini files found in the folder.")
            QMessageBox.critical(self, "Error", "No valid game file (package.json or .ini) found in the selected folder.")

    def launch_nwjs_game(self, folder_path):
        selected_version = self.version_selector.currentText()
        if not selected_version:
            logging.error("No NWJS version selected.")
            QMessageBox.critical(self, "Error", "No NWJS version selected.")
            return

        nwjs_dir = os.path.expanduser(f"~/Library/Application Support/RPGM-Launcher/{selected_version}")
        nwjs_path = os.path.join(nwjs_dir, "nwjs.app/Contents/MacOS/nwjs")

        run_with_rosetta_file = os.path.join(nwjs_dir, "run-with-rosetta")
        if os.path.exists(run_with_rosetta_file):
            subprocess.Popen(["arch", "-x86_64", nwjs_path, folder_path])
        else:
            subprocess.Popen([nwjs_path, folder_path])
        logging.info("Game launched using NWJS version %s.", selected_version)

    def launch_mkxpz_game(self, folder_path):
        if not self.check_mkxpz_installed() or not self.check_RTP_installed():
            return

        mkxpz_json_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/Z-universal.app/Contents/Game/mkxp.json")
        if os.path.exists(mkxpz_json_path):
            try:
                with open(mkxpz_json_path, 'rb') as file:
                    raw_data = file.read()
                    result = chardet.detect(raw_data)
                    encoding = result['encoding']

                with open(mkxpz_json_path, 'r', encoding=encoding) as file:
                    mkxp_config = json.load(file)
                
                mkxp_config["gameFolder"] = folder_path

                rtp_value = self.get_rtp_value(folder_path)
                rtp_path = os.path.join(os.path.expanduser("~/Library/Application Support/RPGM-Launcher/RTP"), rtp_value)
                mkxp_config["RTP"] = [rtp_path]

                with open(mkxpz_json_path, 'w', encoding=encoding) as file:
                    json.dump(mkxp_config, file, indent=4)

                logging.info("Updated mkxp.json with gameFolder: %s and RTP: %s", folder_path, rtp_path)
            except Exception as e:
                logging.error("Failed to update mkxp.json: %s", str(e))
                QMessageBox.critical(self, "Error", f"Failed to update mkxp.json: {str(e)}")
                return
        else:
            logging.error("mkxp.json file not found at %s", mkxpz_json_path)
            QMessageBox.critical(self, "Error", "mkxp.json file not found. MKXPZ launch aborted.")
            return

        mkxpz_executable_path = os.path.expanduser("~/Library/Application Support/RPGM-Launcher/Z-universal.app/Contents/MacOS/Z-universal")
        try:
            subprocess.Popen([mkxpz_executable_path, folder_path])
            logging.info("MKXPZ game launched from folder: %s", folder_path)
        except Exception as e:
            logging.error("Failed to launch MKXPZ: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to launch MKXPZ: {str(e)}")

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

                rtp_value = self.get_rtp_value(folder_path)
                rtp_src = os.path.join(os.path.expanduser("~/Library/Application Support/RPGM-Launcher/RTP"), rtp_value)
                rtp_dst = os.path.join(game_dir, rtp_value)
                shutil.copytree(rtp_src, rtp_dst, dirs_exist_ok=True)

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

            else:
                logging.error("No valid game file (package.json or Game.ini) found in the selected folder.")
                QMessageBox.critical(self, "Error", "No valid game file (package.json or Game.ini) found in the selected folder.")

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
        line_edit.setPlaceholderText("nwjs")
        layout.addWidget(line_edit)

        buttons = QDialogButtonBox(QDialogButtonBox.Ok | QDialogButtonBox.Cancel, dialog)
        buttons.accepted.connect(dialog.accept)
        buttons.rejected.connect(dialog.reject)
        layout.addWidget(buttons)

        result = dialog.exec()
        app_name = line_edit.text().strip() if line_edit.text().strip() else "nwjs"
        return app_name, result == QDialog.Accepted

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
                        download_speed = downloaded_size / (1024 * 1024) / elapsed_time  # MB/s

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

    def install_mkxpz(self):
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
                        download_speed = downloaded_size / (1024 * 1024) / elapsed_time  # MB/s

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
            QMessageBox.information(self, "MKXPZ Installation", "MKXPZ installed successfully.")

        except Exception as e:
            logging.error("Error installing MKXPZ: %s", str(e))
            QMessageBox.critical(self, "Error", f"Failed to install MKXPZ: {str(e)}")
    
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
                        download_speed = downloaded_size / (1024 * 1024) / elapsed_time  # MB/s

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
        versions = [v for v in os.listdir(applications_dir) if os.path.isdir(os.path.join(applications_dir, v))]

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

    def add_cheat_menu(self, folder_path):
        www_folder_path = os.path.join(folder_path, "www")
        cheat_menu_js_path = os.path.join(os.path.dirname(__file__), 'Cheat_Menu.js')
        cheat_menu_css_path = os.path.join(os.path.dirname(__file__), 'Cheat_Menu.css')
        if os.path.exists(www_folder_path) and os.path.isdir(www_folder_path):
            logging.info("The 'www' folder exists in the selected folder.")
            if self.modify_MV_main_js(os.path.join(www_folder_path, 'js', 'main.js')):
                logging.info("RPG MV main.js has been patched with cheat menu.")
                self.copy_cheat_files(www_folder_path)
            else:
                logging.info("RPG MV main.js has already been patched with cheat menu.")
        else:
            logging.warning("The 'www' folder does not exist in the selected folder.")
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
