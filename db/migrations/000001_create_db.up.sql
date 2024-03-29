CREATE TABLE IF NOT EXISTS queueReports (
id INTEGER NOT NULL PRIMARY KEY,
reporter TEXT NOT NULL,
time DATETIME NOT NULL,
queueLength TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS changelogMessages (
id INTEGER NOT NULL PRIMARY KEY,
reporterID INTEGER UNIQUE NOT NULL, 
lastChangelog INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS internetpoints (
id INTEGER NOT NULL PRIMARY KEY,
reporterID INTEGER UNIQUE NOT NULL,
points INTEGER NOT NULL
);
