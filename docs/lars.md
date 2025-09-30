libsql -> heimdak = fil og mappe struktur

1: nyt projekt både med heimdal --project "name" og inde i heimdal project-init name 
Så oprettes udenfor heimdal en project-name.sqlite fil. og inde i heimdal er der nu en mappe /project-name som nu er roden af heimdal-aios-wrapper session 
fremover kan man starte projektet med heimdal project-navn(id) ai-coder og alt vil starte inde i haimdal project-name mappen so. er toppen af sqlite filen 

2: database betjening: Fra heimdal kommando newfile name.type -> new flatfile men navnet name.type og ID og det samme emd mapper en fil er en flatfile hvor hver linje er et felt i faltfile en folder er bare en relations samling af flatfiles 

Alt dette er som det altd har været for AI men den store udvidelse er anniotering og metadata - man kan annotere med // på en linje og det flytter teksten til et side felt i databasen man kan tagge en linje med @@ og det opretteer tags i en tags felt og man kan lægge :: :: og det er AI kommunikaltions DSL feltet AICOM 

De simple ting giver en illusion af en fils truktur men er en miderne relationsdatabase 

3: RAG alle fil-felter af samme type kan ses med RAG og alle filer overhovedet kan ses med RAG og et er et helt område for sig selv med feks alle funktioner med tags eller alle klasser eller tags af en type ideen er at få et intuitivt sprog der gør koden mere DRY så specifik kode nemmere lander under den rette funktion, fil eller klasse alt efter sprog - dette miljø er sprog neutral. det er bare en AI venlig RAG hukommelse 

4: fil eksport 1. som en type samlet 1 fil pr type eller kodebasen som separate filer dette åbner lidt admin af afhængigheder så en lang fil kun har afhængigheder 1 gang og separate filer har dem med. Jeg stoler ikke på at AI har styr på netop det så der skal vi have noget i fremtiden der linter og tilbagenelder ind i heimdal AIOS 
